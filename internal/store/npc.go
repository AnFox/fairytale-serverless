package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/anfox/fairytale-serverless/internal/model"
)

// FindNpcBySheetName is how /npc looks up an NPC by its sheet tab name.
// sheet_id disambiguates NPCs from different spreadsheets that share a tab name.
func (s *Store) FindNpcBySheetName(ctx context.Context, sheetID, sheetName string) (*model.Npc, error) {
	const q = `SELECT id, name, level, hit, damage, crit, current_hp, max_hp,
                      current_mp, max_mp, sheet_id, sheet_name, is_allowed
               FROM npcs WHERE sheet_id = $1 AND sheet_name = $2`
	var n model.Npc
	err := s.pool.QueryRow(ctx, q, sheetID, sheetName).Scan(
		&n.ID, &n.Name, &n.Level, &n.Hit, &n.Damage, &n.Crit,
		&n.CurrentHP, &n.MaxHP, &n.CurrentMP, &n.MaxMP,
		&n.SheetID, &n.SheetName, &n.IsAllowed,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// FindNpcByName finds an NPC by display name (case-insensitive); used when
// user types /npc <name> and we don't have sheet_id resolved yet. Picks the
// most recently updated record when duplicates exist.
func (s *Store) FindNpcByName(ctx context.Context, name string) (*model.Npc, error) {
	const q = `SELECT id, name, level, hit, damage, crit, current_hp, max_hp,
                      current_mp, max_mp, sheet_id, sheet_name, is_allowed
               FROM npcs WHERE LOWER(sheet_name) = LOWER($1) OR LOWER(name) = LOWER($1)
               ORDER BY updated_at DESC NULLS LAST, id DESC LIMIT 1`
	var n model.Npc
	err := s.pool.QueryRow(ctx, q, name).Scan(
		&n.ID, &n.Name, &n.Level, &n.Hit, &n.Damage, &n.Crit,
		&n.CurrentHP, &n.MaxHP, &n.CurrentMP, &n.MaxMP,
		&n.SheetID, &n.SheetName, &n.IsAllowed,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

type NpcSyncTarget struct {
	SheetID   string
	SheetName string
}

// ListNpcSyncTargets returns sheet IDs + tab names the sync loop should pull.
func (s *Store) ListNpcSyncTargets(ctx context.Context) ([]NpcSyncTarget, error) {
	const q = `SELECT DISTINCT sheet_id, sheet_name
               FROM npcs WHERE sheet_id <> '' AND sheet_name <> ''`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NpcSyncTarget
	for rows.Next() {
		var t NpcSyncTarget
		if err := rows.Scan(&t.SheetID, &t.SheetName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpsertNpc writes the sheet-derived stats. Matches on (sheet_id, sheet_name).
func (s *Store) UpsertNpc(ctx context.Context, n model.Npc) error {
	const q = `INSERT INTO npcs
        (name, level, hit, damage, crit, current_hp, max_hp, current_mp, max_mp,
         sheet_id, sheet_name, is_allowed, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW(),NOW())
        ON CONFLICT (sheet_id, sheet_name) DO UPDATE SET
          name=EXCLUDED.name,
          level=EXCLUDED.level,
          hit=EXCLUDED.hit,
          damage=EXCLUDED.damage,
          crit=EXCLUDED.crit,
          current_hp=EXCLUDED.current_hp,
          max_hp=EXCLUDED.max_hp,
          current_mp=EXCLUDED.current_mp,
          max_mp=EXCLUDED.max_mp,
          is_allowed=EXCLUDED.is_allowed,
          updated_at=NOW()`
	_, err := s.pool.Exec(ctx, q,
		n.Name, n.Level, n.Hit, n.Damage, n.Crit,
		n.CurrentHP, n.MaxHP, n.CurrentMP, n.MaxMP,
		n.SheetID, n.SheetName, n.IsAllowed,
	)
	return err
}

// SheetSyncEntry is a row from the hash-tracking table.
type SheetSyncEntry struct {
	SheetName    string
	ContentHash  string
	LastSyncedAt time.Time
}

// GetSheetHash returns the stored hash, or "" when we've never synced it.
func (s *Store) GetSheetHash(ctx context.Context, sheetName string) (string, error) {
	const q = `SELECT content_hash FROM sheet_sync_state WHERE sheet_name = $1`
	var h string
	err := s.pool.QueryRow(ctx, q, sheetName).Scan(&h)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return h, err
}

func (s *Store) SetSheetHash(ctx context.Context, sheetName, hash string) error {
	const q = `INSERT INTO sheet_sync_state (sheet_name, content_hash, last_synced_at)
               VALUES ($1,$2,NOW())
               ON CONFLICT (sheet_name) DO UPDATE SET
                 content_hash=EXCLUDED.content_hash,
                 last_synced_at=NOW()`
	_, err := s.pool.Exec(ctx, q, sheetName, hash)
	return err
}
