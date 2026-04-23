// BotWorker Lambda: consumes raw Telegram updates from BotTasksQueue,
// parses them, runs command/callback/state handlers, and enqueues outgoing
// messages to OutboundQueue.
//
// TODO(port): port handlers from legacy app/Telegram/Handler.php — commands
// (/dice, /npc, weapon numbers), callback-query "repeat", and the 16-state
// conversational machine backed by user_states.
package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/anfox/fairytale-serverless/internal/telegram"
)

func handler(ctx context.Context, ev events.SQSEvent) error {
	for _, rec := range ev.Records {
		var upd telegram.Update
		if err := json.Unmarshal([]byte(rec.Body), &upd); err != nil {
			log.Printf("skip malformed update: %v", err)
			continue
		}
		log.Printf("received update id=%d", upd.UpdateID)
		// TODO: route to command/callback/state handler, enqueue replies.
	}
	return nil
}

func main() {
	lambda.Start(handler)
}
