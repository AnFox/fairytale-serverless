package bot

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/anfox/fairytale-serverless/internal/telegram"
)

// Sender enqueues OutboundMessages onto SQS for the outbound Lambda to send.
// Wrapping SQS in this small interface keeps handlers easy to fake.
type Sender interface {
	Send(ctx context.Context, msg telegram.OutboundMessage) error
}

type SQSSender struct {
	client   *sqs.Client
	queueURL string
}

func NewSQSSender(client *sqs.Client, queueURL string) *SQSSender {
	return &SQSSender{client: client, queueURL: queueURL}
}

func (s *SQSSender) Send(ctx context.Context, msg telegram.OutboundMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal outbound: %w", err)
	}
	_, err = s.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(s.queueURL),
		MessageBody: aws.String(string(body)),
	})
	return err
}
