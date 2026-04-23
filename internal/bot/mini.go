package bot

import (
	"context"
	"fmt"
	"math/rand/v2"

	"github.com/anfox/fairytale-serverless/internal/telegram"
)

// Hardcoded player and NPC rosters mirror the legacy app/Telegram/Handler.php
// w*/drop/i lists. They aren't sourced from the DB on purpose — players come
// and go but the GM uses these commands during sessions to pick someone fast.

var playerRosters = map[string][]string{
	"w":  {"Георгий", "Максим", "Владимир"},
	"wm": {"Георгий", "Владимир"},
	"w4": {"Георгий", "Владимир", "Torvald", "Sven"},
	"w6": {"Георгий", "Максим", "Владимир", "Torvald", "Jukka", "Sven"},
	"w7": {"Георгий", "Максим", "Владимир", "Torvald", "Jukka", "Sven", "Ксандрос"},
}

func (b *Bot) handleWho(ctx context.Context, chatID int64, listKey string) error {
	roster := playerRosters[listKey]
	if len(roster) == 0 {
		return nil
	}
	pick := roster[rand.IntN(len(roster))]
	return b.sender.Send(ctx, telegram.OutboundMessage{
		ChatID: chatID,
		Text:   "🎯 " + pick,
	})
}

// /drop — random item from the basic loot table.
var dropItems = []string{
	"Одноручный меч", "Одноручный топор", "Нож", "Копье",
	"Двуручный меч", "Двуручный топор", "Лук", "Арбалет",
	"Одноручный блант", "Двуручный молот", "Экзотическое оружие",
	"Щит", "Шлем",
	"Легкая броня", "Кольчуга", "Тяжелая броня",
	"Пояс", "Кольцо", "Амулет",
	"Свиток потенциала", "Свиток опыта", "Свиток заклинания",
	"Эликсир",
}

func (b *Bot) handleDrop(ctx context.Context, chatID int64) error {
	pick := dropItems[rand.IntN(len(dropItems))]
	return b.sender.Send(ctx, telegram.OutboundMessage{
		ChatID: chatID,
		Text:   "📦 " + pick,
	})
}

// /i — quality-weighted random item with category and subtype.
//
// Mirrors the legacy /i flow: pick category by uniform die, optionally pick
// subtype for Weapon/Armor/Accessory/Potion, then roll d100 for quality.

var (
	itemTypes = []string{
		"Оружие", "Броня", "Аксессуар", "Зелье",
		"Свиток", "Артефакт", "Драгоценность",
		"Книга", "Контейнер", "Декоративный предмет",
	}
	weaponSubtypes = []string{
		"Одноручный меч", "Одноручный топор", "Кинжал", "Копье",
		"Двуручный меч", "Двуручный топор", "Лук", "Арбалет",
		"Булава", "Двуручный молот", "Метательное", "Экзотическое",
	}
	armorSubtypes     = []string{"Шлем", "Щит", "Плащ", "Легкая броня", "Средняя броня", "Тяжелая броня"}
	accessorySubtypes = []string{"Кольцо", "Амулет", "Серьги", "Браслет", "Пояс", "Корона", "Шарф", "Перчатки"}
	potionSubtypes    = []string{
		"Зелье лечения", "Зелье лечения", "Зелье лечения",
		"Зелье маны", "Зелье маны", "Зелье маны",
		"Зелье силы", "Зелье ловкости", "Зелье выносливости",
		"Зелье интеллекта", "Зелье мудрости", "Зелье харизмы",
		"Зелье невидимости", "Зелье скорости", "Зелье прыжка",
		"Зелье огнестойкости", "Зелье ледостойкости", "Зелье электростойкости",
		"Зелье ядостойкости", "Зелье кислотостойкости",
		"Зелье превращения", "Зелье полета", "Зелье водного дыхания",
		"Зелье ночного зрения", "Масло",
	}
)

// qualityTier holds the upper-bound roll for a quality (out of 100) and its
// emoji. Iterated in order; first tier whose threshold the d100 doesn't
// exceed wins.
type qualityTier struct {
	name      string
	threshold int
	emoji     string
}

var qualityTiers = []qualityTier{
	{"Обычный", 50, "⚪"},
	{"Необычный", 75, "🟢"},
	{"Магический", 90, "🔵"},
	{"Редкий", 97, "🟣"},
	{"Эпический", 99, "🟠"},
	{"Легендарный", 100, "🔴"},
}

func subtypesFor(itemType string) []string {
	switch itemType {
	case "Оружие":
		return weaponSubtypes
	case "Броня":
		return armorSubtypes
	case "Аксессуар":
		return accessorySubtypes
	case "Зелье":
		return potionSubtypes
	}
	return nil
}

func qualityFor(roll100 int) qualityTier {
	for _, t := range qualityTiers {
		if roll100 <= t.threshold {
			return t
		}
	}
	return qualityTiers[len(qualityTiers)-1]
}

func (b *Bot) handleItem(ctx context.Context, msg *telegram.Message) error {
	itemType := itemTypes[rand.IntN(len(itemTypes))]
	subtype := ""
	if subs := subtypesFor(itemType); len(subs) > 0 {
		subtype = subs[rand.IntN(len(subs))]
	}
	q := qualityFor(rand.IntN(100) + 1)

	label := q.emoji + " " + q.name + " "
	if subtype != "" {
		label += subtype
	} else {
		label += itemType
	}

	author := displayName(msg)
	return b.sender.Send(ctx, telegram.OutboundMessage{
		ChatID: msg.Chat.ID,
		Text:   fmt.Sprintf("👤 %s\n📦 %s", author, label),
	})
}
