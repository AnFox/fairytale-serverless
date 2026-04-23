// Command import-mysql reads a mysqldump file and copies the bot-relevant
// rows into Neon Postgres. One-shot tool — not a Lambda — invoked locally.
//
// Usage:
//
//	NEON_DSN='postgres://...' go run ./tools/import-mysql -dump ~/Downloads/fairytale.sql
//
// What it touches:
//   - users, character_classes, characters, weapons, npcs, user_states
//
// Each target table is TRUNCATE'd first so the importer is idempotent.
// Non-target Laravel columns (password, role, google_id, …) are dropped.
// Sequences are reset to MAX(id)+1 after load.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dump := flag.String("dump", "", "path to mysqldump .sql file")
	flag.Parse()
	if *dump == "" {
		log.Fatal("missing -dump path")
	}
	dsn := os.Getenv("NEON_DSN")
	if dsn == "" {
		log.Fatal("NEON_DSN env var required")
	}

	raw, err := os.ReadFile(*dump)
	if err != nil {
		log.Fatalf("read dump: %v", err)
	}
	rows := parseInserts(string(raw))
	log.Printf("parsed %d INSERT statements across %d tables", countTotal(rows), len(rows))

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect neon: %v", err)
	}
	defer pool.Close()

	// Order matters because of FKs.
	steps := []importStep{
		{"users", importUsers},
		{"character_classes", importCharacterClasses},
		{"characters", importCharacters},
		{"weapons", importWeapons},
		{"npcs", importNpcs},
		{"user_states", importUserStates},
	}

	for _, s := range steps {
		n, err := s.fn(ctx, pool, rows[s.table])
		if err != nil {
			log.Fatalf("import %s: %v", s.table, err)
		}
		if err := resetSequence(ctx, pool, s.table); err != nil {
			log.Fatalf("reset seq %s: %v", s.table, err)
		}
		log.Printf("  %s: %d rows", s.table, n)
	}

	log.Println("done")
}

type importStep struct {
	table string
	fn    func(ctx context.Context, pool *pgxpool.Pool, rows [][]string) (int, error)
}

func countTotal(m map[string][][]string) int {
	n := 0
	for _, v := range m {
		n += len(v)
	}
	return n
}

// parseInserts collects raw VALUES tuples per table from a mysqldump file.
// Returns map[table][rowIndex][colIndex]string. Quoted values keep their
// MySQL escapes (\\', \\\\); column-specific importers reinterpret as needed.
func parseInserts(sql string) map[string][][]string {
	stmtRE := regexp.MustCompile(`(?i)INSERT INTO ` + "`" + `([^` + "`" + `]+)` + "`" + ` VALUES \((.*)\);`)
	out := map[string][][]string{}
	for _, line := range strings.Split(sql, "\n") {
		m := stmtRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		table := m[1]
		fields := splitMysqlValues(m[2])
		out[table] = append(out[table], fields)
	}
	return out
}

// splitMysqlValues splits a single VALUES tuple like
//
//	1, 'Bob', NULL, 'has \'apostrophe\''
//
// into raw string fields. Single-quoted strings keep their surrounding quotes
// so callers can detect NULL vs empty-string. NULL stays as the literal "NULL".
func splitMysqlValues(s string) []string {
	var out []string
	var cur strings.Builder
	inStr := false
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escape {
			cur.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' && inStr {
			cur.WriteByte(c)
			escape = true
			continue
		}
		if c == '\'' {
			cur.WriteByte(c)
			inStr = !inStr
			continue
		}
		if c == ',' && !inStr {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}

// nullable returns nil for "NULL" else the unquoted/unescaped string.
func nullable(s string) any {
	if s == "NULL" {
		return nil
	}
	return unquote(s)
}

// nonNull returns the unquoted string or "" for NULL — for NOT NULL columns.
func nonNull(s string) string {
	if s == "NULL" {
		return ""
	}
	return unquote(s)
}

// unquote strips surrounding 's and reverses MySQL backslash escapes.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		s = s[1 : len(s)-1]
	}
	s = strings.ReplaceAll(s, `\'`, `'`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\r`, "\r")
	s = strings.ReplaceAll(s, `\t`, "\t")
	return s
}

// numOrNull returns the raw numeric token, or nil for NULL.
func numOrNull(s string) any {
	if s == "NULL" {
		return nil
	}
	return s
}

func resetSequence(ctx context.Context, pool *pgxpool.Pool, table string) error {
	q := fmt.Sprintf(
		`SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE(MAX(id), 1)) FROM %s`,
		table, table,
	)
	_, err := pool.Exec(ctx, q)
	return err
}

func truncateAndCopy(ctx context.Context, pool *pgxpool.Pool, table, insertSQL string, rows [][]any) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)); err != nil {
		return 0, fmt.Errorf("truncate: %w", err)
	}

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(insertSQL, r...)
	}
	br := tx.SendBatch(ctx, batch)
	for range rows {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return 0, fmt.Errorf("insert: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// MySQL columns: id, name, email, google_id, role, telegram_id,
// current_weapon_number, sheet, spreadsheet_id, email_verified_at,
// password, remember_token, created_at, updated_at.
// We keep: id, name, email, telegram_id, current_weapon_number,
// spreadsheet_id, sheet, created_at, updated_at.
func importUsers(ctx context.Context, pool *pgxpool.Pool, rows [][]string) (int, error) {
	const sql = `INSERT INTO users
        (id, name, email, telegram_id, current_weapon_number, spreadsheet_id, sheet, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`

	// telegram_id is UNIQUE in our schema. Legacy data has duplicates
	// (a person who later signed in via web with the same TG account).
	// First occurrence wins; duplicates keep their record with telegram_id NULL.
	seen := map[string]bool{}
	var batch [][]any
	for _, r := range rows {
		tg := numOrNull(r[5])
		if s, ok := tg.(string); ok && s != "" {
			if seen[s] {
				log.Printf("  users: duplicate telegram_id %s on user id=%s — nulling", s, r[0])
				tg = nil
			} else {
				seen[s] = true
			}
		}
		batch = append(batch, []any{
			numOrNull(r[0]),  // id
			nonNull(r[1]),    // name
			nullable(r[2]),   // email
			tg,               // telegram_id (deduped)
			numOrNull(r[6]),  // current_weapon_number
			nullable(r[8]),   // spreadsheet_id
			nullable(r[7]),   // sheet
			nullable(r[12]),  // created_at
			nullable(r[13]),  // updated_at
		})
	}
	return truncateAndCopy(ctx, pool, "users", sql, batch)
}

// character_classes: id, name, parent_class_id, min_level, max_level, created_at, updated_at — 1:1.
func importCharacterClasses(ctx context.Context, pool *pgxpool.Pool, rows [][]string) (int, error) {
	const sql = `INSERT INTO character_classes
        (id, name, parent_class_id, min_level, max_level, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7)`
	var batch [][]any
	for _, r := range rows {
		batch = append(batch, []any{
			numOrNull(r[0]), nonNull(r[1]), numOrNull(r[2]),
			numOrNull(r[3]), numOrNull(r[4]),
			nullable(r[5]), nullable(r[6]),
		})
	}
	return truncateAndCopy(ctx, pool, "character_classes", sql, batch)
}

// MySQL characters: id, user_id, class_id, name, avatar, level, hp,
// current_mp, max_mp, mp, ac, armor, pp, exp, gold, str, con, dex, int,
// wis, chr, created_at, updated_at.
// Postgres skips avatar.
func importCharacters(ctx context.Context, pool *pgxpool.Pool, rows [][]string) (int, error) {
	const sql = `INSERT INTO characters
        (id, user_id, class_id, name, level, hp, mp, current_mp, max_mp,
         ac, armor, pp, exp, gold, str, con, dex, int, wis, chr, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`
	var batch [][]any
	for _, r := range rows {
		batch = append(batch, []any{
			numOrNull(r[0]),   // id
			numOrNull(r[1]),   // user_id
			numOrNull(r[2]),   // class_id
			nonNull(r[3]),     // name
			numOrNull(r[5]),   // level
			numOrNull(r[6]),   // hp
			numOrNull(r[9]),   // mp
			numOrNull(r[7]),   // current_mp
			numOrNull(r[8]),   // max_mp
			numOrNull(r[10]),  // ac
			numOrNull(r[11]),  // armor
			numOrNull(r[12]),  // pp
			numOrNull(r[13]),  // exp
			numOrNull(r[14]),  // gold
			numOrNull(r[15]),  // str
			numOrNull(r[16]),  // con
			numOrNull(r[17]),  // dex
			numOrNull(r[18]),  // int
			numOrNull(r[19]),  // wis
			numOrNull(r[20]),  // chr
			nullable(r[21]),   // created_at
			nullable(r[22]),   // updated_at
		})
	}
	return truncateAndCopy(ctx, pool, "characters", sql, batch)
}

// weapons: id, user_id, number, name, hit, damage, crit, created_at, updated_at — 1:1.
func importWeapons(ctx context.Context, pool *pgxpool.Pool, rows [][]string) (int, error) {
	const sql = `INSERT INTO weapons
        (id, user_id, number, name, hit, damage, crit, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	var batch [][]any
	for _, r := range rows {
		batch = append(batch, []any{
			numOrNull(r[0]), numOrNull(r[1]), numOrNull(r[2]),
			nullable(r[3]), nonNull(r[4]), nonNull(r[5]),
			numOrNull(r[6]),
			nullable(r[7]), nullable(r[8]),
		})
	}
	return truncateAndCopy(ctx, pool, "weapons", sql, batch)
}

// MySQL npcs: id, name, avatar, level, hit, damage, crit,
// current_hp, max_hp, current_mp, max_mp, sheet_id, sheet_name,
// created_at, updated_at, is_allowed.
// Postgres skips avatar.
func importNpcs(ctx context.Context, pool *pgxpool.Pool, rows [][]string) (int, error) {
	const sql = `INSERT INTO npcs
        (id, name, level, hit, damage, crit, current_hp, max_hp,
         current_mp, max_mp, sheet_id, sheet_name, is_allowed, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`
	var batch [][]any
	for _, r := range rows {
		isAllowed := false
		if v := nonNull(r[15]); v == "1" {
			isAllowed = true
		}
		batch = append(batch, []any{
			numOrNull(r[0]),  // id
			nonNull(r[1]),    // name
			numOrNull(r[3]),  // level (skip avatar at 2)
			nonNull(r[4]),    // hit
			nonNull(r[5]),    // damage
			numOrNull(r[6]),  // crit
			numOrNull(r[7]),  // current_hp
			numOrNull(r[8]),  // max_hp
			numOrNull(r[9]),  // current_mp
			numOrNull(r[10]), // max_mp
			nonNull(r[11]),   // sheet_id
			nonNull(r[12]),   // sheet_name
			isAllowed,
			nullable(r[13]),  // created_at
			nullable(r[14]),  // updated_at
		})
	}
	return truncateAndCopy(ctx, pool, "npcs", sql, batch)
}

// user_states: id, chat_id, user_id, state, created_at, updated_at — Postgres
// adds expires_at (NULL).
func importUserStates(ctx context.Context, pool *pgxpool.Pool, rows [][]string) (int, error) {
	const sql = `INSERT INTO user_states
        (id, chat_id, user_id, state, expires_at, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7)`
	var batch [][]any
	for _, r := range rows {
		batch = append(batch, []any{
			numOrNull(r[0]), numOrNull(r[1]), numOrNull(r[2]),
			nonNull(r[3]),
			nil, // expires_at
			nullable(r[4]), nullable(r[5]),
		})
	}
	return truncateAndCopy(ctx, pool, "user_states", sql, batch)
}
