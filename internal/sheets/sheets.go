// Package sheets is a tiny Google Sheets v4 client that reads public
// spreadsheets (shared as "anyone with the link") using just an API key.
// No OAuth, no service account — one HTTP GET per fetch. Keeps the
// sheetssync Lambda dependency-light and cold-starts fast.
package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const apiBase = "https://sheets.googleapis.com/v4/spreadsheets"

type Client struct {
	apiKey string
	http   *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

// Get fetches a single A1-notation range as a 2D string grid. Sheet name with
// spaces or Cyrillic letters works because we URL-encode it.
func (c *Client) Get(ctx context.Context, spreadsheetID, sheetName, a1Range string) ([][]string, error) {
	rng := sheetName + "!" + a1Range
	endpoint := fmt.Sprintf(
		"%s/%s/values/%s?key=%s",
		apiBase,
		url.PathEscape(spreadsheetID),
		url.PathEscape(rng),
		url.QueryEscape(c.apiKey),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sheets get: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("sheets %s: %s", resp.Status, string(body))
	}

	var payload struct {
		Values [][]any `json:"values"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse sheets response: %w", err)
	}

	// Flatten to strings. Sheets API may return numbers/bools natively —
	// we coerce everything to their %v print form because downstream code
	// parses these cells anyway.
	out := make([][]string, len(payload.Values))
	for i, row := range payload.Values {
		cells := make([]string, len(row))
		for j, v := range row {
			if v == nil {
				continue
			}
			cells[j] = fmt.Sprintf("%v", v)
		}
		out[i] = cells
	}
	return out, nil
}
