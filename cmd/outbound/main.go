// Outbound Lambda: consumes OutboundQueue and sends messages via Telegram
// Bot API. Separated from botworker so outgoing traffic can be throttled
// or retried independently of command processing.
package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/anfox/fairytale-serverless/internal/config"
	"github.com/anfox/fairytale-serverless/internal/telegram"
)

var (
	initOnce sync.Once
	cfg      *config.Config
	tg       *telegram.Client
	initErr  error
)

func warmInit(ctx context.Context) error {
	initOnce.Do(func() {
		cfg, initErr = config.Load(ctx, config.KeyTelegramBotToken)
		if initErr != nil {
			return
		}
		tg = telegram.NewClient(cfg.TelegramBotToken)
	})
	return initErr
}

func handler(ctx context.Context, ev events.SQSEvent) error {
	if err := warmInit(ctx); err != nil {
		return err
	}

	for _, rec := range ev.Records {
		var msg telegram.OutboundMessage
		if err := json.Unmarshal([]byte(rec.Body), &msg); err != nil {
			log.Printf("skip malformed outbound: %v", err)
			continue
		}
		if msg.CallbackQueryID != "" {
			if err := tg.AnswerCallbackQuery(ctx, telegram.AnswerCallbackQueryRequest{
				CallbackQueryID: msg.CallbackQueryID,
				Text:            msg.CallbackText,
			}); err != nil {
				return err
			}
		}
		if msg.Text == "" {
			continue
		}
		if err := tg.SendMessage(ctx, telegram.SendMessageRequest{
			ChatID:      msg.ChatID,
			Text:        msg.Text,
			ParseMode:   msg.ParseMode,
			ReplyMarkup: msg.Keyboard,
		}); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	lambda.Start(handler)
}
