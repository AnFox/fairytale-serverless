// SheetsSync Lambda: triggered by EventBridge every 5 minutes. Pulls
// character sheets and NPC sheets from Google, hash-skips when a tab hasn't
// changed since last run, and upserts into Neon when it has.
//
// One Sheets API call per tab is cheap — ~6 today (5 player tabs + 1 NPC
// spreadsheet with multiple tabs). The hash-skip in Postgres keeps the
// average case to "fetch + compare", no writes to Neon.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/anfox/fairytale-serverless/internal/config"
	"github.com/anfox/fairytale-serverless/internal/database"
	"github.com/anfox/fairytale-serverless/internal/sheets"
	"github.com/anfox/fairytale-serverless/internal/store"
)

const fetchRange = "A1:H17"

type Result struct {
	Characters SyncCount `json:"characters"`
	Npcs       SyncCount `json:"npcs"`
}

type SyncCount struct {
	Checked int `json:"checked"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Errored int `json:"errored"`
}

func handler(ctx context.Context) (Result, error) {
	cfg, err := config.Load(ctx, config.KeyNeonDSN, config.KeyGoogleDeveloperKey)
	if err != nil {
		return Result{}, fmt.Errorf("load config: %w", err)
	}
	db, err := database.New(ctx, cfg.NeonDSN)
	if err != nil {
		return Result{}, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	s := store.New(db.Pool)
	sheetsCli := sheets.NewClient(cfg.GoogleDeveloperKey)

	var res Result
	res.Characters = syncCharacters(ctx, s, sheetsCli)
	res.Npcs = syncNpcs(ctx, s, sheetsCli)
	log.Printf("done: %+v", res)
	return res, nil
}

// outcome lets the per-sheet helper signal which counter to bump.
type outcome int

const (
	outcomeUpdated outcome = iota
	outcomeSkipped
)

func syncCharacters(ctx context.Context, s *store.Store, sc *sheets.Client) SyncCount {
	var c SyncCount
	targets, err := s.ListCharacterSyncTargets(ctx)
	if err != nil {
		log.Printf("list character targets: %v", err)
		return c
	}
	for _, t := range targets {
		c.Checked++
		key := fmt.Sprintf("character:%d:%s", t.UserID, t.SheetName)
		oc, err := syncOneCharacter(ctx, s, sc, t, key)
		switch {
		case err != nil:
			log.Printf("character user=%d sheet=%q: %v", t.UserID, t.SheetName, err)
			c.Errored++
		case oc == outcomeUpdated:
			c.Updated++
		default:
			c.Skipped++
		}
	}
	return c
}

func syncOneCharacter(ctx context.Context, s *store.Store, sc *sheets.Client, t store.CharacterSyncTarget, key string) (outcome, error) {
	grid, err := sc.Get(ctx, t.SpreadsheetID, t.SheetName, fetchRange)
	if err != nil {
		return outcomeSkipped, fmt.Errorf("fetch: %w", err)
	}
	hash := sheets.HashGrid(grid)
	prev, err := s.GetSheetHash(ctx, key)
	if err != nil {
		return outcomeSkipped, fmt.Errorf("get hash: %w", err)
	}
	if prev == hash {
		return outcomeSkipped, nil
	}

	character, weapons := sheets.ParseCharacterSheet(grid, t.UserID)
	if character.Name == "" {
		// An empty name means the sheet hasn't been filled yet; skip without
		// writing so we don't blow away an existing record with blanks.
		log.Printf("character user=%d sheet=%q: empty name, skipping write", t.UserID, t.SheetName)
		return outcomeSkipped, s.SetSheetHash(ctx, key, hash)
	}
	if err := s.UpsertCharacter(ctx, character); err != nil {
		return outcomeSkipped, fmt.Errorf("upsert character: %w", err)
	}

	// Upsert present slots; the legacy importer deletes vacated rows but slot
	// churn is rare and we don't have the previous slot list cheaply, so we
	// leave existing rows alone — matches the imported-data invariant.
	for _, w := range weapons {
		if err := s.UpsertWeapon(ctx, w); err != nil {
			return outcomeSkipped, fmt.Errorf("upsert weapon %d: %w", w.Number, err)
		}
	}
	if err := s.SetSheetHash(ctx, key, hash); err != nil {
		return outcomeSkipped, fmt.Errorf("set hash: %w", err)
	}
	log.Printf("character user=%d sheet=%q: updated (%d weapons)", t.UserID, t.SheetName, len(weapons))
	return outcomeUpdated, nil
}

func syncNpcs(ctx context.Context, s *store.Store, sc *sheets.Client) SyncCount {
	var c SyncCount
	targets, err := s.ListNpcSyncTargets(ctx)
	if err != nil {
		log.Printf("list npc targets: %v", err)
		return c
	}
	for _, t := range targets {
		c.Checked++
		key := fmt.Sprintf("npc:%s:%s", t.SheetID, t.SheetName)
		oc, err := syncOneNpc(ctx, s, sc, t, key)
		switch {
		case err != nil:
			log.Printf("npc sheet=%q: %v", t.SheetName, err)
			c.Errored++
		case oc == outcomeUpdated:
			c.Updated++
		default:
			c.Skipped++
		}
	}
	return c
}

func syncOneNpc(ctx context.Context, s *store.Store, sc *sheets.Client, t store.NpcSyncTarget, key string) (outcome, error) {
	grid, err := sc.Get(ctx, t.SheetID, t.SheetName, fetchRange)
	if err != nil {
		return outcomeSkipped, fmt.Errorf("fetch: %w", err)
	}
	hash := sheets.HashGrid(grid)
	prev, err := s.GetSheetHash(ctx, key)
	if err != nil {
		return outcomeSkipped, fmt.Errorf("get hash: %w", err)
	}
	if prev == hash {
		return outcomeSkipped, nil
	}
	npc := sheets.ParseNpcSheet(grid, t.SheetID, t.SheetName, 1)
	if npc.Name == "" {
		log.Printf("npc sheet=%q: empty name, skipping write", t.SheetName)
		return outcomeSkipped, s.SetSheetHash(ctx, key, hash)
	}
	if err := s.UpsertNpc(ctx, npc); err != nil {
		return outcomeSkipped, fmt.Errorf("upsert npc: %w", err)
	}
	if err := s.SetSheetHash(ctx, key, hash); err != nil {
		return outcomeSkipped, fmt.Errorf("set hash: %w", err)
	}
	log.Printf("npc sheet=%q: updated", t.SheetName)
	return outcomeUpdated, nil
}

func main() {
	lambda.Start(handler)
}
