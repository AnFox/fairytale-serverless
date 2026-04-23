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

// AdminUserID is the legacy hard-coded admin (user.id == 1) — only this user
// may roll NPCs that are not is_allowed. Mirrors Handler::npc().
const AdminUserID = 1

// handleNpc parses the /npc command tail and rolls the requested NPC.
//
// Forms:
//   /npc <name>            -- roll first weapon, no AC
//   /npc <name> <ac>       -- with target AC
func (b *Bot) handleNpc(ctx context.Context, msg *telegram.Message, args string) error {
	parts := strings.Fields(strings.TrimSpace(args))
	if len(parts) == 0 {
		return b.sender.Send(ctx, telegram.OutboundMessage{
			ChatID: msg.Chat.ID,
			Text:   "Укажи имя NPC: /npc <name> [ac]",
		})
	}
	name := parts[0]
	ac := 0
	if len(parts) >= 2 {
		if v, err := strconv.Atoi(parts[1]); err == nil {
			ac = v
		}
	}

	// Resolve the caller so we can let the admin roll restricted NPCs.
	isAdmin := false
	if msg.From != nil {
		if u, err := b.store.FindUserByTelegramID(ctx, msg.From.ID); err == nil && u.ID == AdminUserID {
			isAdmin = true
		}
	}
	return b.rollNpcByName(ctx, msg.Chat.ID, name, ac, isAdmin)
}

func (b *Bot) rollNpcByName(ctx context.Context, chatID int64, name string, ac int, isAdmin bool) error {
	npc, err := b.store.FindNpcByName(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return b.sender.Send(ctx, telegram.OutboundMessage{
			ChatID: chatID,
			Text:   "NPC не найден",
		})
	}
	if err != nil {
		return fmt.Errorf("find npc: %w", err)
	}
	if !npc.IsAllowed && !isAdmin {
		// Match legacy behavior: silently ignore — players shouldn't even
		// know hidden NPCs exist by name.
		return nil
	}
	return b.rollNpcWeapon(ctx, chatID, npc, ac)
}

func (b *Bot) rollNpcWeapon(ctx context.Context, chatID int64, npc *model.Npc, ac int) error {
	hit := dice.Parse(npc.Hit).Execute().ApplyCrit(npc.Crit)
	dmg := dice.Parse(npc.Damage).Execute()

	res := computeWeaponOutcome(hit, dmg, ac)
	text := formatNpcMessage(npc, ac, res)
	kb := repeatKeyboard(callbackData{
		Action:  "rerollNpc",
		NpcName: npc.SheetName,
		AC:      ac,
	})

	return b.sender.Send(ctx, telegram.OutboundMessage{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "Markdown",
		Keyboard:  kb,
	})
}

func formatNpcMessage(npc *model.Npc, ac int, res weaponOutcome) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "👾 %s (lvl %d) \\[1]", npc.Name, npc.Level)
	fmt.Fprintf(&sb, "\n🎲 Попадание: %s", npc.Hit)
	fmt.Fprintf(&sb, "\n       *%s*", res.HitOutput)
	if ac > 0 {
		fmt.Fprintf(&sb, "\n🛡 AC: %d", ac)
	}
	if res.DamageRoll != nil {
		fmt.Fprintf(&sb, "\n⚔️ Урон: %s\n       *%s*", npc.Damage, res.DamageBlock)
	}
	return sb.String()
}
