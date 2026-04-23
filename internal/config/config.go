// Package config loads runtime configuration for Lambda functions.
//
// Secrets are stored in SSM Parameter Store under /fairytale/{stage}/*.
// Non-secret config is passed via Lambda env vars (set in template.yaml).
package config

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type Config struct {
	Stage                 string
	Region                string
	SSMPrefix             string
	TelegramBotToken      string
	TelegramWebhookSecret string
	NeonDSN               string
	GoogleServiceAccount  string
	GoogleDeveloperKey    string
	BotTasksQueueURL      string
	OutboundQueueURL      string
}

// Load fetches SSM parameters and reads env vars. Fetch only what's needed
// by passing the relevant keys — each Lambda cares about a different subset.
func Load(ctx context.Context, keys ...string) (*Config, error) {
	stage := os.Getenv("STAGE")
	if stage == "" {
		stage = "dev"
	}
	region := os.Getenv("REGION")
	if region == "" {
		region = "eu-central-1"
	}
	prefix := os.Getenv("SSM_PREFIX")
	if prefix == "" {
		prefix = "/fairytale/" + stage
	}

	cfg := &Config{
		Stage:            stage,
		Region:           region,
		SSMPrefix:        prefix,
		BotTasksQueueURL: os.Getenv("BOT_TASKS_QUEUE_URL"),
		OutboundQueueURL: os.Getenv("OUTBOUND_QUEUE_URL"),
	}

	if len(keys) == 0 {
		return cfg, nil
	}

	awsCfg, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	client := ssm.NewFromConfig(awsCfg)

	names := make([]string, 0, len(keys))
	for _, k := range keys {
		names = append(names, prefix+"/"+k)
	}

	out, err := client.GetParameters(ctx, &ssm.GetParametersInput{
		Names:          names,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("ssm get: %w", err)
	}
	if len(out.InvalidParameters) > 0 {
		return nil, fmt.Errorf("missing ssm params: %s", strings.Join(out.InvalidParameters, ", "))
	}

	for _, p := range out.Parameters {
		applyParam(cfg, prefix, p)
	}
	return cfg, nil
}

func applyParam(cfg *Config, prefix string, p types.Parameter) {
	key := strings.TrimPrefix(aws.ToString(p.Name), prefix+"/")
	v := aws.ToString(p.Value)
	switch key {
	case "telegram-bot-token":
		cfg.TelegramBotToken = v
	case "telegram-webhook-secret":
		cfg.TelegramWebhookSecret = v
	case "neon-dsn":
		cfg.NeonDSN = v
	case "google-service-account":
		cfg.GoogleServiceAccount = v
	case "google-developer-key":
		cfg.GoogleDeveloperKey = v
	}
}

// Keys are the SSM parameter keys (relative to SSMPrefix).
const (
	KeyTelegramBotToken      = "telegram-bot-token"
	KeyTelegramWebhookSecret = "telegram-webhook-secret"
	KeyNeonDSN               = "neon-dsn"
	KeyGoogleServiceAccount  = "google-service-account"
	KeyGoogleDeveloperKey    = "google-developer-key"
)
