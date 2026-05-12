// SheetsSync Lambda: triggered by EventBridge every 5 minutes.
//
// Goal: don't touch Neon on the steady-state tick. State lives in S3
// (sync state bucket → state.json) and contains both the list of sheets
// to pull and the last-seen hash per sheet. Each tick:
//
//  1. Load state.json from S3 (or build a fresh one from Neon on first run).
//  2. Fetch every sheet listed in state, compute current hashes.
//  3. If all hashes match — return. Neon was never opened.
//  4. Otherwise — open Neon, re-read targets (catches new users/NPCs),
//     parse + upsert changed sheets, write the new state.json.
//
// A manual `aws lambda invoke ... --payload '{"force":true}'` short-circuits
// to the slow path even when nothing changed — used to pick up new users
// without waiting for a sheet edit.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/anfox/fairytale-serverless/internal/config"
	"github.com/anfox/fairytale-serverless/internal/database"
	"github.com/anfox/fairytale-serverless/internal/sheets"
	"github.com/anfox/fairytale-serverless/internal/store"
	"github.com/anfox/fairytale-serverless/internal/syncstate"
)

const fetchRange = "A1:H17"

// Event is the optional payload — EventBridge sends an empty object, manual
// invokes can set Force to bypass the hash-skip path.
type Event struct {
	Force bool `json:"force,omitempty"`
}

type Result struct {
	Path       string `json:"path"`        // "skip", "slow", "first-run"
	SheetsRead int    `json:"sheets_read"` // total Google Sheets fetches
	Updated    int    `json:"updated"`     // sheets we actually upserted
}

func handler(ctx context.Context, ev Event) (Result, error) {
	cfg, err := config.Load(ctx, config.KeyNeonDSN, config.KeyGoogleDeveloperKey)
	if err != nil {
		return Result{}, fmt.Errorf("load config: %w", err)
	}
	bucket := os.Getenv("SYNC_STATE_BUCKET")
	if bucket == "" {
		return Result{}, fmt.Errorf("SYNC_STATE_BUCKET env not set")
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(cfg.Region))
	if err != nil {
		return Result{}, fmt.Errorf("load aws config: %w", err)
	}

	sheetsCli := sheets.NewClient(cfg.GoogleDeveloperKey)
	ssStore := syncstate.New(s3.NewFromConfig(awsCfg), bucket)

	state, err := ssStore.Load(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("load state: %w", err)
	}

	// First run / forced refresh → straight to the slow path.
	if state.IsEmpty() || ev.Force {
		log.Printf("entering slow path (empty=%v force=%v)", state.IsEmpty(), ev.Force)
		return runSlowPath(ctx, cfg.NeonDSN, sheetsCli, ssStore)
	}

	// Fast path: fetch every known sheet, compute hashes, compare.
	current, grids, err := fetchAndHash(ctx, sheetsCli, state)
	if err != nil {
		return Result{}, fmt.Errorf("fast path fetch: %w", err)
	}

	if state.EqualHashes(current) {
		log.Printf("path=skip sheets_read=%d", len(current))
		return Result{Path: "skip", SheetsRead: len(current)}, nil
	}

	log.Printf("hashes changed; entering slow path")
	return runSlowWithGrids(ctx, cfg.NeonDSN, sheetsCli, ssStore, state, current, grids)
}

// fetchAndHash pulls every sheet listed in state and returns both the hash
// map and the raw grids (so the slow path doesn't have to refetch).
func fetchAndHash(
	ctx context.Context, sc *sheets.Client, st syncstate.State,
) (map[string]string, map[string][][]string, error) {
	hashes := make(map[string]string, len(st.Hashes))
	grids := make(map[string][][]string, len(st.Hashes))
	for _, t := range st.Characters {
		key := characterKey(t.UserID, t.SheetName)
		grid, err := sc.Get(ctx, t.SpreadsheetID, t.SheetName, fetchRange)
		if err != nil {
			return nil, nil, fmt.Errorf("char user=%d sheet=%q: %w", t.UserID, t.SheetName, err)
		}
		hashes[key] = hashGrid(grid)
		grids[key] = grid
	}
	for _, t := range st.Npcs {
		key := npcKey(t.SheetID, t.SheetName)
		grid, err := sc.Get(ctx, t.SheetID, t.SheetName, fetchRange)
		if err != nil {
			return nil, nil, fmt.Errorf("npc sheet=%q: %w", t.SheetName, err)
		}
		hashes[key] = hashGrid(grid)
		grids[key] = grid
	}
	return hashes, grids, nil
}

// runSlowPath opens Neon, re-reads targets, fetches everything, upserts, and
// writes the new state. Used on first run / forced refresh.
func runSlowPath(
	ctx context.Context, dsn string, sc *sheets.Client, ssStore *syncstate.Store,
) (Result, error) {
	db, err := database.New(ctx, dsn)
	if err != nil {
		return Result{}, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	s := store.New(db.Pool)

	chars, err := s.ListCharacterSyncTargets(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list character targets: %w", err)
	}
	npcs, err := s.ListNpcSyncTargets(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list npc targets: %w", err)
	}

	// Build a state object from Neon, fetch every sheet, then sync.
	state := syncstate.State{
		Characters: make([]syncstate.CharacterTarget, len(chars)),
		Npcs:       make([]syncstate.NpcTarget, len(npcs)),
		Hashes:     map[string]string{},
	}
	for i, c := range chars {
		state.Characters[i] = syncstate.CharacterTarget{
			UserID:        c.UserID,
			SpreadsheetID: c.SpreadsheetID,
			SheetName:     c.SheetName,
		}
	}
	for i, n := range npcs {
		state.Npcs[i] = syncstate.NpcTarget{
			SheetID: n.SheetID, SheetName: n.SheetName,
		}
	}

	current, grids, err := fetchAndHash(ctx, sc, state)
	if err != nil {
		return Result{}, err
	}
	state.Hashes = current

	updated, err := upsertChanged(ctx, s, state, grids, map[string]string{})
	if err != nil {
		return Result{}, err
	}
	if err := ssStore.Save(ctx, state); err != nil {
		return Result{}, fmt.Errorf("save state: %w", err)
	}
	log.Printf("path=first-run sheets_read=%d updated=%d", len(current), updated)
	return Result{Path: "first-run", SheetsRead: len(current), Updated: updated}, nil
}

// runSlowWithGrids is the slow path when the fast path already fetched grids
// and we just need to refresh targets + upsert + save. Re-reads targets from
// Neon to catch new users/NPCs that appeared since last save.
func runSlowWithGrids(
	ctx context.Context, dsn string, sc *sheets.Client, ssStore *syncstate.Store,
	prev syncstate.State, current map[string]string, grids map[string][][]string,
) (Result, error) {
	db, err := database.New(ctx, dsn)
	if err != nil {
		return Result{}, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	s := store.New(db.Pool)

	chars, err := s.ListCharacterSyncTargets(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list character targets: %w", err)
	}
	npcs, err := s.ListNpcSyncTargets(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list npc targets: %w", err)
	}

	// Fetch any sheets that are in Neon but weren't in the previous state
	// (new user, new NPC). Their grids aren't in our hand yet.
	newState := syncstate.State{
		Hashes: map[string]string{},
	}
	for _, c := range chars {
		newState.Characters = append(newState.Characters, syncstate.CharacterTarget{
			UserID: c.UserID, SpreadsheetID: c.SpreadsheetID, SheetName: c.SheetName,
		})
		key := characterKey(c.UserID, c.SheetName)
		if _, ok := grids[key]; !ok {
			grid, err := sc.Get(ctx, c.SpreadsheetID, c.SheetName, fetchRange)
			if err != nil {
				return Result{}, fmt.Errorf("fetch new char user=%d: %w", c.UserID, err)
			}
			grids[key] = grid
			current[key] = hashGrid(grid)
		}
		newState.Hashes[key] = current[key]
	}
	for _, n := range npcs {
		newState.Npcs = append(newState.Npcs, syncstate.NpcTarget{
			SheetID: n.SheetID, SheetName: n.SheetName,
		})
		key := npcKey(n.SheetID, n.SheetName)
		if _, ok := grids[key]; !ok {
			grid, err := sc.Get(ctx, n.SheetID, n.SheetName, fetchRange)
			if err != nil {
				return Result{}, fmt.Errorf("fetch new npc sheet=%q: %w", n.SheetName, err)
			}
			grids[key] = grid
			current[key] = hashGrid(grid)
		}
		newState.Hashes[key] = current[key]
	}

	updated, err := upsertChanged(ctx, s, newState, grids, prev.Hashes)
	if err != nil {
		return Result{}, err
	}
	if err := ssStore.Save(ctx, newState); err != nil {
		return Result{}, fmt.Errorf("save state: %w", err)
	}
	log.Printf("path=slow sheets_read=%d updated=%d", len(current), updated)
	return Result{Path: "slow", SheetsRead: len(current), Updated: updated}, nil
}

// upsertChanged writes characters/weapons/NPCs to Neon for every sheet whose
// hash differs from the previous state. prev is empty on first run, which
// causes everything to be treated as changed.
func upsertChanged(
	ctx context.Context, s *store.Store, st syncstate.State,
	grids map[string][][]string, prev map[string]string,
) (int, error) {
	updated := 0
	for _, t := range st.Characters {
		key := characterKey(t.UserID, t.SheetName)
		if prev[key] == st.Hashes[key] {
			continue
		}
		grid := grids[key]
		character, weapons := sheets.ParseCharacterSheet(grid, t.UserID)
		if character.Name == "" {
			log.Printf("character user=%d sheet=%q: empty name, skipping upsert", t.UserID, t.SheetName)
			continue
		}
		if err := s.UpsertCharacter(ctx, character); err != nil {
			return updated, fmt.Errorf("upsert character user=%d: %w", t.UserID, err)
		}
		for _, w := range weapons {
			if err := s.UpsertWeapon(ctx, w); err != nil {
				return updated, fmt.Errorf("upsert weapon user=%d num=%d: %w", t.UserID, w.Number, err)
			}
		}
		log.Printf("character user=%d sheet=%q: updated (%d weapons)", t.UserID, t.SheetName, len(weapons))
		updated++
	}
	for _, t := range st.Npcs {
		key := npcKey(t.SheetID, t.SheetName)
		if prev[key] == st.Hashes[key] {
			continue
		}
		grid := grids[key]
		npc := sheets.ParseNpcSheet(grid, t.SheetID, t.SheetName, 1)
		if npc.Name == "" {
			log.Printf("npc sheet=%q: empty name, skipping upsert", t.SheetName)
			continue
		}
		if err := s.UpsertNpc(ctx, npc); err != nil {
			return updated, fmt.Errorf("upsert npc sheet=%q: %w", t.SheetName, err)
		}
		log.Printf("npc sheet=%q: updated", t.SheetName)
		updated++
	}
	return updated, nil
}

func hashGrid(grid [][]string) string {
	b, _ := json.Marshal(grid)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func characterKey(userID int64, sheetName string) string {
	return fmt.Sprintf("character:%d:%s", userID, sheetName)
}

func npcKey(sheetID, sheetName string) string {
	return fmt.Sprintf("npc:%s:%s", sheetID, sheetName)
}

func main() {
	lambda.Start(handler)
}
