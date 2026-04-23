# fairytale-serverless

Serverless Telegram bot for tabletop RPG dice rolls and character management.
Migration target from the legacy Laravel monolith — scope is **bot + Google Sheets sync only**.

## Stack
- Go 1.25, Lambda ARM64 (`provided.al2023`)
- AWS SAM (eu-central-1)
- Neon Postgres 18 (eu-central-1)
- API Gateway REST (Telegram webhook)
- SQS (bot tasks, outbound) + DLQ
- EventBridge (Sheets sync, 5 min)
- SSM Parameter Store (secrets)

## Layout
```
cmd/
  webhook/       # Telegram webhook → enqueue to BotTasksQueue
  botworker/     # SQS consumer: parse command, roll dice, update state, enqueue outbound
  outbound/      # SQS consumer: send message to Telegram Bot API
  sheetssync/    # EventBridge-triggered: pull sheets, skip if hash unchanged
  migrate/       # Invoked manually to apply SQL migrations
internal/
  bot/           # command routing + handlers
  config/        # SSM loader
  database/      # pgx pool wrapper
  dice/          # NdM±K parser + roller
  sheets/        # Google Sheets client + hash-skip
  state/         # user_states machine
  store/         # repositories (users, characters, weapons, ...)
  telegram/      # Telegram Bot API client
migrations/      # *.sql applied by cmd/migrate
template.yaml
Makefile
```

## Setup (one-time)

### SSM parameters

```bash
# Replace <values>
aws ssm put-parameter --name /fairytale/dev/telegram-bot-token --value "<bot_token>" --type SecureString --region eu-central-1
aws ssm put-parameter --name /fairytale/dev/telegram-webhook-secret --value "<random_secret>" --type SecureString --region eu-central-1
aws ssm put-parameter --name /fairytale/dev/neon-dsn --value "postgres://...pooler.../neondb?sslmode=require" --type SecureString --region eu-central-1
aws ssm put-parameter --name /fairytale/dev/google-service-account --value "<service_account_json>" --type SecureString --region eu-central-1
```

### Deploy

```bash
make build
make deploy-dev
make migrate-dev
make set-webhook-dev
```

## Commands

```bash
make build              # sam build
make deploy-dev         # deploy to eu-central-1 (stack: fairytale-dev)
make deploy-prod        # deploy to eu-central-1 (stack: fairytale-prod)
make migrate-dev        # invoke migrate Lambda on dev
make logs-webhook       # tail CloudWatch logs
make logs-botworker
make dlq-check          # inspect DLQ depth
make set-webhook-dev    # point Telegram bot webhook at API Gateway URL
make test               # go test ./...
```
