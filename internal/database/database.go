// Package database wraps pgxpool with Lambda-friendly defaults.
//
// Use a small pool (a Lambda container handles one request at a time) and
// connect through the Neon pooler endpoint so we don't hit connection limits.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Client struct {
	Pool *pgxpool.Pool
}

// New opens the pool and verifies connectivity.
func New(ctx context.Context, dsn string) (*Client, error) {
	if dsn == "" {
		return nil, fmt.Errorf("empty dsn")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Client{Pool: pool}, nil
}

func (c *Client) Close() {
	if c.Pool != nil {
		c.Pool.Close()
	}
}
