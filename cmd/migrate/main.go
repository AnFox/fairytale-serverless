// Migrate Lambda: applies embedded SQL migrations to Neon.
// Invoked manually via `make migrate-dev` / `make migrate-prod`.
package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/anfox/fairytale-serverless/internal/config"
	"github.com/anfox/fairytale-serverless/internal/database"
	"github.com/anfox/fairytale-serverless/internal/migrator"
)

func handler(ctx context.Context) (migrator.Result, error) {
	cfg, err := config.Load(ctx, config.KeyNeonDSN)
	if err != nil {
		return migrator.Result{}, fmt.Errorf("load config: %w", err)
	}
	db, err := database.New(ctx, cfg.NeonDSN)
	if err != nil {
		return migrator.Result{}, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	return migrator.Apply(ctx, db.Pool)
}

func main() {
	lambda.Start(handler)
}
