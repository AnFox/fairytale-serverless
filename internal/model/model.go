// Package model contains plain domain structs shared across packages.
// Repositories in internal/store marshal to/from these.
package model

import "time"

type User struct {
	ID                   int64
	Name                 string
	Email                *string
	TelegramID           *int64
	CurrentWeaponNumber  *int
	SpreadsheetID        *string
	Sheet                *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Character struct {
	ID        int64
	UserID    int64
	ClassID   *int64
	Name      string
	Level     int
	HP        int
	MP        int
	CurrentMP *int
	MaxMP     *int
	AC        int
	Armor     int
	PP        int
	Exp       int
	Gold      int
	Str       int
	Con       int
	Dex       int
	Int       int
	Wis       int
	Chr       int
}

type Weapon struct {
	ID     int64
	UserID int64
	Number int
	Name   *string
	Hit    string
	Damage string
	Crit   int
}

type UserState struct {
	ID        int64
	ChatID    int64
	UserID    int64
	State     string
	ExpiresAt *time.Time
}

type Npc struct {
	ID        int64
	Name      string
	Level     int
	Hit       string
	Damage    string
	Crit      int
	CurrentHP *int
	MaxHP     *int
	CurrentMP *int
	MaxMP     *int
	SheetID   string
	SheetName string
	IsAllowed bool
}
