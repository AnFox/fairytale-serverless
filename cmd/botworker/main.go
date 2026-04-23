// BotWorker Lambda: consumes raw Telegram updates from BotTasksQueue,
// parses them, runs command/callback handlers, and enqueues outgoing
// messages onto OutboundQueue.
package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/anfox/fairytale-serverless/internal/bot"
	"github.com/anfox/fairytale-serverless/internal/config"
	"github.com/anfox/fairytale-serverless/internal/database"
	"github.com/anfox/fairytale-serverless/internal/store"
	"github.com/anfox/fairytale-serverless/internal/telegram"
)

var (
	initOnce sync.Once
	app      *bot.Bot
	db       *database.Client
	initErr  error
)

func warmInit(ctx context.Context) error {
	initOnce.Do(func() {
		cfg, err := config.Load(ctx, config.KeyNeonDSN)
		if err != nil {
			initErr = err
			return
		}
		db, initErr = database.New(ctx, cfg.NeonDSN)
		if initErr != nil {
			return
		}
		awsCfg, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(cfg.Region))
		if err != nil {
			initErr = err
			return
		}
		sender := bot.NewSQSSender(sqs.NewFromConfig(awsCfg), cfg.OutboundQueueURL)
		app = bot.New(store.New(db.Pool), sender)
	})
	return initErr
}

func handler(ctx context.Context, ev events.SQSEvent) error {
	if err := warmInit(ctx); err != nil {
		return err
	}

	for _, rec := range ev.Records {
		var upd telegram.Update
		if err := json.Unmarshal([]byte(rec.Body), &upd); err != nil {
			log.Printf("skip malformed update: %v", err)
			continue
		}
		logUpdate(upd)
		if err := app.Handle(ctx, upd); err != nil {
			// Returning an error fails the SQS batch (single-message batch),
			// triggering retry → DLQ after maxReceiveCount.
			return err
		}
	}
	return nil
}

func logUpdate(u telegram.Update) {
	switch {
	case u.CallbackQuery != nil:
		log.Printf("update %d: callback from=%d data=%q", u.UpdateID, u.CallbackQuery.From.ID, u.CallbackQuery.Data)
	case u.Message != nil:
		from := int64(0)
		if u.Message.From != nil {
			from = u.Message.From.ID
		}
		log.Printf("update %d: message from=%d chat=%d text=%q", u.UpdateID, from, u.Message.Chat.ID, u.Message.Text)
	default:
		log.Printf("update %d: unhandled type", u.UpdateID)
	}
}

func main() {
	lambda.Start(handler)
}
