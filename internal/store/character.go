package store

import (
	"context"

	"github.com/anfox/fairytale-serverless/internal/model"
)

// CharacterSyncTarget is what sheetssync pulls from: the user + their tab.
type CharacterSyncTarget struct {
	UserID        int64
	SpreadsheetID string
	SheetName     string
}

// ListCharacterSyncTargets returns users with a sheet configured so we know
// whose sheet to pull. Skips users where telegram_id is NULL or sheet is NULL.
func (s *Store) ListCharacterSyncTargets(ctx context.Context) ([]CharacterSyncTarget, error) {
	const q = `SELECT id, COALESCE(spreadsheet_id, ''), COALESCE(sheet, '')
               FROM users
               WHERE sheet IS NOT NULL AND sheet <> ''
                 AND spreadsheet_id IS NOT NULL AND spreadsheet_id <> ''`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CharacterSyncTarget
	for rows.Next() {
		var t CharacterSyncTarget
		if err := rows.Scan(&t.UserID, &t.SpreadsheetID, &t.SheetName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpsertCharacter refreshes a character's sheet-derived stats. Matches on
// (user_id, name) so an existing record is updated rather than duplicated.
// If the user has no character yet, the first sheet sync creates one.
func (s *Store) UpsertCharacter(ctx context.Context, c model.Character) error {
	const q = `INSERT INTO characters
        (user_id, class_id, name, level, hp, mp, current_mp, max_mp,
         ac, armor, pp, exp, gold, str, con, dex, int, wis, chr,
         created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,NOW(),NOW())
        ON CONFLICT (user_id, name) DO UPDATE SET
          class_id=EXCLUDED.class_id,
          level=EXCLUDED.level,
          hp=EXCLUDED.hp,
          mp=EXCLUDED.mp,
          current_mp=EXCLUDED.current_mp,
          max_mp=EXCLUDED.max_mp,
          ac=EXCLUDED.ac,
          armor=EXCLUDED.armor,
          pp=EXCLUDED.pp,
          exp=EXCLUDED.exp,
          gold=EXCLUDED.gold,
          str=EXCLUDED.str,
          con=EXCLUDED.con,
          dex=EXCLUDED.dex,
          int=EXCLUDED.int,
          wis=EXCLUDED.wis,
          chr=EXCLUDED.chr,
          updated_at=NOW()`
	_, err := s.pool.Exec(ctx, q,
		c.UserID, c.ClassID, c.Name, c.Level, c.HP, c.MP, c.CurrentMP, c.MaxMP,
		c.AC, c.Armor, c.PP, c.Exp, c.Gold,
		c.Str, c.Con, c.Dex, c.Int, c.Wis, c.Chr,
	)
	return err
}

// UpsertWeapon writes/updates by (user_id, number) — the same slot-based key
// the bot uses when a player types a digit to roll.
func (s *Store) UpsertWeapon(ctx context.Context, w model.Weapon) error {
	const q = `INSERT INTO weapons
        (user_id, number, name, hit, damage, crit, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,NOW(),NOW())
        ON CONFLICT (user_id, number) DO UPDATE SET
          name=EXCLUDED.name,
          hit=EXCLUDED.hit,
          damage=EXCLUDED.damage,
          crit=EXCLUDED.crit,
          updated_at=NOW()`
	_, err := s.pool.Exec(ctx, q,
		w.UserID, w.Number, w.Name, w.Hit, w.Damage, w.Crit,
	)
	return err
}

// DeleteWeapon removes a weapon slot. sheetssync calls this when a row the
// user used to have is now empty on the sheet.
func (s *Store) DeleteWeapon(ctx context.Context, userID int64, number int) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM weapons WHERE user_id = $1 AND number = $2`, userID, number)
	return err
}
