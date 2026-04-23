// Webhook Lambda: receives Telegram updates via API Gateway, verifies the
// secret header, and enqueues the raw update to BotTasksQueue for async
// processing. Must respond within ~1s (Telegram retries after 5s).
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

	"github.com/anfox/fairytale-serverless/internal/config"
)

const telegramSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

var (
	initOnce sync.Once
	cfg      *config.Config
	sqsCli   *sqs.Client
	initErr  error
)

func warmInit(ctx context.Context) error {
	initOnce.Do(func() {
		cfg, initErr = config.Load(ctx, config.KeyTelegramWebhookSecret)
		if initErr != nil {
			return
		}
		awsCfg, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(cfg.Region))
		if err != nil {
			initErr = err
			return
		}
		sqsCli = sqs.NewFromConfig(awsCfg)
	})
	return initErr
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if err := warmInit(ctx); err != nil {
		log.Printf("init error: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 500}, nil
	}

	// Header match (API Gateway normalizes header case inconsistently).
	got := ""
	for k, v := range req.Headers {
		if equalFold(k, telegramSecretHeader) {
			got = v
			break
		}
	}
	if got == "" || got != cfg.TelegramWebhookSecret {
		log.Println("rejected webhook: bad secret token")
		return events.APIGatewayProxyResponse{StatusCode: 401}, nil
	}

	// Sanity-check that the body is JSON; we don't parse the Update — the
	// worker does that. We just need to enqueue it as-is.
	if !json.Valid([]byte(req.Body)) {
		log.Println("rejected webhook: invalid json body")
		return events.APIGatewayProxyResponse{StatusCode: 400}, nil
	}

	body := req.Body
	_, err := sqsCli.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &cfg.BotTasksQueueURL,
		MessageBody: &body,
	})
	if err != nil {
		log.Printf("sqs send error: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 500}, nil
	}

	return events.APIGatewayProxyResponse{StatusCode: 200, Body: "ok"}, nil
}

func main() {
	lambda.Start(handler)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
