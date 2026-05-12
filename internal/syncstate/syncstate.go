// Package syncstate stores everything sheetssync needs to skip Neon on a
// no-changes tick: the list of sheets to pull (so we don't have to read
// users/npcs from Neon) and the per-sheet content hashes from the last
// successful run. Backing store is a single JSON object on S3.
//
// State is refreshed from Neon only when a change is detected (slow path)
// or when explicitly forced via the Lambda invocation payload. New users
// or NPCs added to Neon while every sheet is unchanged stay invisible
// until either a slow-path run or a forced refresh.
package syncstate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const objectKey = "state.json"

// CharacterTarget mirrors store.CharacterSyncTarget but is JSON-shaped.
type CharacterTarget struct {
	UserID        int64  `json:"user_id"`
	SpreadsheetID string `json:"spreadsheet_id"`
	SheetName     string `json:"sheet_name"`
}

// NpcTarget mirrors store.NpcSyncTarget but is JSON-shaped.
type NpcTarget struct {
	SheetID   string `json:"sheet_id"`
	SheetName string `json:"sheet_name"`
}

// State is the whole blob stored on S3.
type State struct {
	Characters []CharacterTarget `json:"characters"`
	Npcs       []NpcTarget       `json:"npcs"`
	Hashes     map[string]string `json:"hashes"`
}

// IsEmpty returns true when there's nothing to sync yet — first run, or the
// state file was deleted. Callers should fall back to a Neon read.
func (s State) IsEmpty() bool {
	return len(s.Characters) == 0 && len(s.Npcs) == 0
}

// EqualHashes returns true iff `current` matches the saved hashes for every
// key we know about. Missing keys count as a difference so a brand-new
// sheet always triggers the slow path.
func (s State) EqualHashes(current map[string]string) bool {
	if len(s.Hashes) != len(current) {
		return false
	}
	for k, v := range s.Hashes {
		if current[k] != v {
			return false
		}
	}
	return true
}

type Store struct {
	client *s3.Client
	bucket string
}

func New(client *s3.Client, bucket string) *Store {
	return &Store{client: client, bucket: bucket}
}

// Load fetches the saved state. Returns a zero-value State (not an error)
// when the object doesn't exist yet — first deploy hasn't written anything.
func (s *Store) Load(ctx context.Context) (State, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return State{Hashes: map[string]string{}}, nil
		}
		return State{}, fmt.Errorf("s3 get %s: %w", objectKey, err)
	}
	defer out.Body.Close()
	body, err := io.ReadAll(out.Body)
	if err != nil {
		return State{}, fmt.Errorf("read body: %w", err)
	}
	var st State
	if err := json.Unmarshal(body, &st); err != nil {
		return State{}, fmt.Errorf("unmarshal state: %w", err)
	}
	if st.Hashes == nil {
		st.Hashes = map[string]string{}
	}
	return st, nil
}

// Save writes the full state. PUT is atomic.
func (s *Store) Save(ctx context.Context, st State) error {
	body, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", objectKey, err)
	}
	return nil
}
