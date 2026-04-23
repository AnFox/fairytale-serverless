// Package store hosts pgx-backed repositories for the bot domain.
// Each repo is a thin wrapper over the pool so handlers can stay testable
// (swap a fake Store) and the SQL stays close to the read site.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anfox/fairytale-serverless/internal/model"
)

var ErrNotFound = errors.New("store: not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// FindUserByTelegramID returns ErrNotFound if no user has this telegram_id.
func (s *Store) FindUserByTelegramID(ctx context.Context, tgID int64) (*model.User, error) {
	const q = `SELECT id, name, email, telegram_id, current_weapon_number, spreadsheet_id, sheet, created_at, updated_at
               FROM users WHERE telegram_id = $1`
	var u model.User
	err := s.pool.QueryRow(ctx, q, tgID).Scan(
		&u.ID, &u.Name, &u.Email, &u.TelegramID, &u.CurrentWeaponNumber,
		&u.SpreadsheetID, &u.Sheet, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FirstCharacterByUserID returns the user's primary character (lowest id).
// In the legacy app each user has exactly one character; this matches that.
func (s *Store) FirstCharacterByUserID(ctx context.Context, userID int64) (*model.Character, error) {
	const q = `SELECT id, user_id, class_id, name, level, hp, mp, current_mp, max_mp,
                      ac, armor, pp, exp, gold, str, con, dex, int, wis, chr
               FROM characters WHERE user_id = $1 ORDER BY id LIMIT 1`
	var c model.Character
	err := s.pool.QueryRow(ctx, q, userID).Scan(
		&c.ID, &c.UserID, &c.ClassID, &c.Name, &c.Level, &c.HP, &c.MP,
		&c.CurrentMP, &c.MaxMP, &c.AC, &c.Armor, &c.PP, &c.Exp, &c.Gold,
		&c.Str, &c.Con, &c.Dex, &c.Int, &c.Wis, &c.Chr,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// FindWeapon by user id + slot number.
func (s *Store) FindWeapon(ctx context.Context, userID int64, number int) (*model.Weapon, error) {
	const q = `SELECT id, user_id, number, name, hit, damage, crit
               FROM weapons WHERE user_id = $1 AND number = $2`
	var w model.Weapon
	err := s.pool.QueryRow(ctx, q, userID, number).Scan(
		&w.ID, &w.UserID, &w.Number, &w.Name, &w.Hit, &w.Damage, &w.Crit,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}
