package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anfox/fairytale-serverless/internal/store"
	"github.com/anfox/fairytale-serverless/internal/telegram"
)

// callbackData is the JSON payload we put in inline button callback_data.
// Telegram caps callback_data at 64 bytes, so keep field names tight.
type callbackData struct {
	Action  string `json:"a"`
	Formula string `json:"f,omitempty"`
	Number  int    `json:"n,omitempty"`
	AC      int    `json:"ac,omitempty"`
}

func encodeCallback(cd callbackData) string {
	b, _ := json.Marshal(cd)
	return string(b)
}

func decodeCallback(s string) (callbackData, error) {
	var cd callbackData
	err := json.Unmarshal([]byte(s), &cd)
	return cd, err
}

func repeatKeyboard(cd callbackData) *telegram.InlineKeyboardMarkup {
	return &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "повторить", CallbackData: encodeCallback(cd)},
		}},
	}
}

func (b *Bot) handleCallback(ctx context.Context, q *telegram.CallbackQuery) error {
	cd, err := decodeCallback(q.Data)
	if err != nil {
		// Acknowledge unknown payloads so the spinner stops.
		return b.sender.Send(ctx, telegram.OutboundMessage{
			CallbackQueryID: q.ID,
		})
	}

	// Always ack the callback so Telegram clears the spinner.
	if err := b.sender.Send(ctx, telegram.OutboundMessage{CallbackQueryID: q.ID}); err != nil {
		return err
	}

	if q.Message == nil {
		return nil
	}
	chatID := q.Message.Chat.ID
	author := q.From.FirstName

	switch cd.Action {
	case "reroll":
		return b.handleDiceFormula(ctx, &telegram.Message{
			Chat: q.Message.Chat,
			From: &q.From,
			Text: cd.Formula,
		}, cd.Formula, author)

	case "rerollWeapon":
		user, err := b.store.FindUserByTelegramID(ctx, q.From.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find user: %w", err)
		}
		return b.rollWeaponFor(ctx, chatID, user, cd.Number, cd.AC)

	default:
		return nil
	}
}
