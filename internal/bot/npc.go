package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/anfox/fairytale-serverless/internal/dice"
	"github.com/anfox/fairytale-serverless/internal/model"
	"github.com/anfox/fairytale-serverless/internal/sheets"
	"github.com/anfox/fairytale-serverless/internal/store"
	"github.com/anfox/fairytale-serverless/internal/telegram"
)

// AdminUserID is the legacy hard-coded admin (user.id == 1) — only this user
// may roll NPCs that are not is_allowed. Mirrors Handler::npc().
const AdminUserID = 1

// npcFetchRange mirrors the A1:H17 range the sheetssync Lambda reads; the
// NPC parser assumes this layout.
const npcFetchRange = "A1:H17"

// handleNpc parses /npc <name> [weapon_number] [ac] — matching the legacy
// syntax. Missing args default to weapon_number=1, ac=0.
func (b *Bot) handleNpc(ctx context.Context, msg *telegram.Message, args string) error {
	parts := strings.Fields(strings.TrimSpace(args))
	if len(parts) == 0 {
		return b.sender.Send(ctx, telegram.OutboundMessage{
			ChatID: msg.Chat.ID,
			Text:   "Укажи имя NPC: /npc <name> [weapon] [ac]",
		})
	}
	name := parts[0]
	weaponNumber := 1
	ac := 0
	if len(parts) >= 2 {
		if v, err := strconv.Atoi(parts[1]); err == nil && v > 0 {
			weaponNumber = v
		}
	}
	if len(parts) >= 3 {
		if v, err := strconv.Atoi(parts[2]); err == nil {
			ac = v
		}
	}

	isAdmin := false
	if msg.From != nil {
		if u, err := b.store.FindUserByTelegramID(ctx, msg.From.ID); err == nil && u.ID == AdminUserID {
			isAdmin = true
		}
	}
	return b.rollNpcByName(ctx, msg.Chat.ID, name, weaponNumber, ac, isAdmin)
}

// rollNpcByName resolves the NPC and picks the right data source:
//   - weapon #1 is read from Neon (the primary weapon sheetssync keeps warm)
//   - other weapons are fetched fresh from Google Sheets, since we only cache
//     the primary. Matches the legacy "fetch + cache for 30s" flow closely
//     enough for small playgroups.
func (b *Bot) rollNpcByName(ctx context.Context, chatID int64, name string, weaponNumber, ac int, isAdmin bool) error {
	stored, err := b.store.FindNpcByName(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return b.sender.Send(ctx, telegram.OutboundMessage{ChatID: chatID, Text: "NPC не найден"})
	}
	if err != nil {
		return fmt.Errorf("find npc: %w", err)
	}
	if !stored.IsAllowed && !isAdmin {
		// Match legacy behavior: silently ignore hidden NPCs.
		return nil
	}

	npc := stored
	if weaponNumber != 1 {
		fresh, err := b.fetchNpcWeapon(ctx, stored, weaponNumber)
		if err != nil {
			return b.sender.Send(ctx, telegram.OutboundMessage{
				ChatID: chatID,
				Text:   fmt.Sprintf("Не удалось получить оружие %d для %s", weaponNumber, stored.Name),
			})
		}
		npc = fresh
	}
	return b.rollNpcWeapon(ctx, chatID, npc, weaponNumber, ac)
}

// fetchNpcWeapon pulls the NPC's sheet and parses weapon #weaponNumber live.
// Returns a non-persisted Npc struct; caller just uses it for the roll.
func (b *Bot) fetchNpcWeapon(ctx context.Context, base *model.Npc, weaponNumber int) (*model.Npc, error) {
	if b.sheets == nil {
		return nil, errors.New("sheets client not configured")
	}
	grid, err := b.sheets.Get(ctx, base.SheetID, base.SheetName, npcFetchRange)
	if err != nil {
		return nil, err
	}
	fresh := sheets.ParseNpcSheet(grid, base.SheetID, base.SheetName, weaponNumber)
	// Preserve the authoritative Name from Neon so /npc replies with a stable
	// label even if the sheet cell is blank.
	if fresh.Name == "" {
		fresh.Name = base.Name
	}
	return &fresh, nil
}

func (b *Bot) rollNpcWeapon(ctx context.Context, chatID int64, npc *model.Npc, weaponNumber, ac int) error {
	hit := dice.Parse(npc.Hit).Execute().ApplyCrit(npc.Crit)
	dmg := dice.Parse(npc.Damage).Execute()

	res := computeWeaponOutcome(hit, dmg, ac)
	text := formatNpcMessage(npc, weaponNumber, ac, res)
	kb := repeatKeyboard(callbackData{
		Action:  "rerollNpc",
		NpcName: npc.SheetName,
		Number:  weaponNumber,
		AC:      ac,
	})

	return b.sender.Send(ctx, telegram.OutboundMessage{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "Markdown",
		Keyboard:  kb,
	})
}

func formatNpcMessage(npc *model.Npc, weaponNumber, ac int, res weaponOutcome) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "👾 %s (lvl %d) \\[%d]", npc.Name, npc.Level, weaponNumber)
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
