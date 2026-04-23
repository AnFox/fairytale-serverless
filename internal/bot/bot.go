// Package bot routes Telegram updates to handlers and enqueues replies via
// the Sender. Stateless commands and weapon rolls live here; the conversational
// state machine and registration flow are added in a later pass.
package bot

import (
	"context"
	"log"
	"strconv"
	"strings"
	"unicode"

	"github.com/anfox/fairytale-serverless/internal/sheets"
	"github.com/anfox/fairytale-serverless/internal/store"
	"github.com/anfox/fairytale-serverless/internal/telegram"
)

type Bot struct {
	store  *store.Store
	sender Sender
	sheets *sheets.Client // optional; nil disables fresh-sheet lookups
}

func New(s *store.Store, sender Sender, sc *sheets.Client) *Bot {
	return &Bot{store: s, sender: sender, sheets: sc}
}

// Handle dispatches an inbound Telegram update.
func (b *Bot) Handle(ctx context.Context, upd telegram.Update) error {
	switch {
	case upd.CallbackQuery != nil:
		return b.handleCallback(ctx, upd.CallbackQuery)
	case upd.Message != nil:
		return b.handleMessage(ctx, upd.Message)
	default:
		log.Printf("update %d: nothing to handle", upd.UpdateID)
		return nil
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *telegram.Message) error {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return nil
	}

	switch {
	case strings.HasPrefix(text, "/"):
		return b.handleCommand(ctx, msg, text)
	case isInteger(text):
		n, _ := strconv.Atoi(text)
		return b.handleWeaponRoll(ctx, msg, n, 0)
	case looksLikeFormula(text):
		return b.handleDiceFormula(ctx, msg, text, displayName(msg))
	default:
		// Unrecognized text — ignore for now (state machine will plug here later).
		return nil
	}
}

// looksLikeFormula matches strings the dice parser will accept, e.g. "d20",
// "2d6+3". Plain prose is intentionally rejected so we don't reply to chat.
func looksLikeFormula(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return false
	}
	hasD := false
	for _, r := range s {
		switch {
		case unicode.IsDigit(r), r == '+', r == '-':
			// allowed
		case r == 'd':
			hasD = true
		default:
			return false
		}
	}
	return hasD
}

func isInteger(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func displayName(msg *telegram.Message) string {
	if msg.From == nil {
		return ""
	}
	if msg.From.FirstName != "" {
		return msg.From.FirstName
	}
	if msg.From.Username != "" {
		return msg.From.Username
	}
	return ""
}
