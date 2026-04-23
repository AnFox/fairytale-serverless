package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/anfox/fairytale-serverless/internal/dice"
	"github.com/anfox/fairytale-serverless/internal/model"
	"github.com/anfox/fairytale-serverless/internal/store"
	"github.com/anfox/fairytale-serverless/internal/telegram"
)

// handleCommand dispatches /-prefixed commands. Unknown commands stay silent
// so we don't spam channels the bot is added to.
func (b *Bot) handleCommand(ctx context.Context, msg *telegram.Message, text string) error {
	cmd, _, _ := strings.Cut(strings.TrimPrefix(text, "/"), " ")
	cmd = strings.SplitN(cmd, "@", 2)[0] // strip @botusername in groups
	cmd = strings.ToLower(cmd)

	switch cmd {
	case "ping":
		return b.sender.Send(ctx, telegram.OutboundMessage{
			ChatID: msg.Chat.ID,
			Text:   "pong",
		})
	case "start":
		return b.sender.Send(ctx, telegram.OutboundMessage{
			ChatID: msg.Chat.ID,
			Text:   "Hello-hello!",
		})
	case "d":
		// Telegram's animated 🎲 — same as legacy $chat->dice()->send().
		return b.sender.Send(ctx, telegram.OutboundMessage{
			ChatID: msg.Chat.ID,
			Dice:   true,
		})
	case "dice":
		// "/dice 2d6+3" — strip the command, treat tail as formula.
		_, tail, _ := strings.Cut(text, " ")
		tail = strings.TrimSpace(tail)
		if tail == "" {
			return nil
		}
		return b.handleDiceFormula(ctx, msg, tail, displayName(msg))
	case "npc":
		_, tail, _ := strings.Cut(text, " ")
		return b.handleNpc(ctx, msg, tail)
	case "w", "wm", "w4", "w6", "w7":
		return b.handleWho(ctx, msg.Chat.ID, cmd)
	case "drop":
		return b.handleDrop(ctx, msg.Chat.ID)
	case "i":
		return b.handleItem(ctx, msg)
	default:
		// "/3" or "/3 12" — slash-prefixed weapon roll. Tail (if present)
		// is parsed as target AC.
		if isInteger(cmd) {
			n, _ := strconv.Atoi(cmd)
			ac := 0
			_, tail, _ := strings.Cut(text, " ")
			if tail = strings.TrimSpace(tail); tail != "" {
				if v, err := strconv.Atoi(tail); err == nil {
					ac = v
				}
			}
			return b.handleWeaponRoll(ctx, msg, n, ac)
		}
		// "/d6", "/d20", "/2d6+3" — bare formulas as commands.
		if looksLikeFormula(cmd) {
			return b.handleDiceFormula(ctx, msg, cmd, displayName(msg))
		}
		return nil
	}
}

// handleDiceFormula rolls a free-form formula and replies with a "повторить" button.
func (b *Bot) handleDiceFormula(ctx context.Context, msg *telegram.Message, formula, author string) error {
	parsed := dice.Parse(formula)
	if parsed.Dice == 0 {
		return nil
	}
	roll := parsed.Execute().ApplyCrit(20)

	out := formatDiceMessage(author, roll)
	kb := repeatKeyboard(callbackData{Action: "reroll", Formula: roll.Input})

	return b.sender.Send(ctx, telegram.OutboundMessage{
		ChatID:    msg.Chat.ID,
		Text:      out,
		ParseMode: "Markdown",
		Keyboard:  kb,
	})
}

// handleWeaponRoll resolves the user by telegram_id, looks up the weapon,
// rolls hit + damage, and formats the response with crit/miss/AC logic.
func (b *Bot) handleWeaponRoll(ctx context.Context, msg *telegram.Message, number, ac int) error {
	if msg.From == nil {
		return nil
	}
	user, err := b.store.FindUserByTelegramID(ctx, msg.From.ID)
	if errors.Is(err, store.ErrNotFound) {
		return b.sender.Send(ctx, telegram.OutboundMessage{
			ChatID: msg.Chat.ID,
			Text:   "Тебя нет в базе. Попроси GM добавить тебя.",
		})
	}
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	return b.rollWeaponFor(ctx, msg.Chat.ID, user, number, ac)
}

func (b *Bot) rollWeaponFor(ctx context.Context, chatID int64, user *model.User, number, ac int) error {
	weapon, err := b.store.FindWeapon(ctx, user.ID, number)
	if errors.Is(err, store.ErrNotFound) {
		return b.sender.Send(ctx, telegram.OutboundMessage{
			ChatID: chatID,
			Text:   "Оружие не найдено",
		})
	}
	if err != nil {
		return fmt.Errorf("find weapon: %w", err)
	}

	// Damage formula may reference STR/CON/etc — substitute from character if present.
	damageFormula := weapon.Damage
	if char, err := b.store.FirstCharacterByUserID(ctx, user.ID); err == nil {
		damageFormula = substituteAttrs(damageFormula, char)
	}

	hit := dice.Parse(weapon.Hit).Execute().ApplyCrit(weapon.Crit)
	dmg := dice.Parse(damageFormula).Execute()

	res := computeWeaponOutcome(hit, dmg, ac)

	text := formatWeaponMessage(user.Name, weapon, number, ac, res)
	kb := repeatKeyboard(callbackData{Action: "rerollWeapon", Number: number, AC: ac})

	return b.sender.Send(ctx, telegram.OutboundMessage{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "Markdown",
		Keyboard:  kb,
	})
}
