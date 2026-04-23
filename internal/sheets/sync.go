package sheets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/anfox/fairytale-serverless/internal/model"
)

// The sheet layout matches the legacy Laravel app/Services/SheetsService.php:
// column A holds labels, column B holds primary values, and columns D-H hold
// weapon rows starting at row 3 (index 2). See ParseCharacterSheet below.
//
// A1:H17 is the full range the legacy app fetched — same grid is used for
// characters and NPCs, but column mappings diverge (see ParseNpcSheet).

const WeaponsPerCharacter = 5

// HashGrid returns a stable sha256 over the grid's contents so sheetssync can
// skip a sheet when nothing changed. JSON ensures ragged rows and embedded
// separators don't collide.
func HashGrid(grid [][]string) string {
	b, _ := json.Marshal(grid)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ParseCharacterSheet turns a character sheet into a Character + up to five
// weapons ready to upsert. userID is required because nothing in the sheet
// carries it. Returns a non-nil Character even if the sheet is sparse —
// sheetssync decides whether to skip upsert based on hash, not content.
func ParseCharacterSheet(grid [][]string, userID int64) (model.Character, []model.Weapon) {
	c := model.Character{
		UserID: userID,
		Name:   Cell(grid, 0, 1),
		Str:    NumOrDefault(Cell(grid, 2, 1), 1),
		Con:    NumOrDefault(Cell(grid, 3, 1), 1),
		Dex:    NumOrDefault(Cell(grid, 4, 1), 1),
		Int:    NumOrDefault(Cell(grid, 5, 1), 1),
		Wis:    NumOrDefault(Cell(grid, 6, 1), 1),
		Chr:    NumOrDefault(Cell(grid, 7, 1), 1),
		Level:  NumOrDefault(Cell(grid, 9, 1), 0),
	}

	// HP accepts "12/30" or just "30"; we store only the max as c.HP.
	if _, maxHP := ParseCurrentMax(Cell(grid, 10, 1)); maxHP != nil {
		c.HP = *maxHP
	}
	// MP supports both the old single value and the newer "current/max" form.
	if cur, maxMP := ParseCurrentMax(Cell(grid, 11, 1)); maxMP != nil {
		c.CurrentMP = cur
		c.MaxMP = maxMP
		c.MP = *maxMP
	}

	c.AC = NumOrDefault(Cell(grid, 12, 1), 10)
	c.Armor = NumOrDefault(Cell(grid, 13, 1), 0)
	c.PP = NumOrDefault(Cell(grid, 14, 1), 0)
	// Exp cell may be "current/max"; keep only current.
	if cur, _ := ParseCurrentMax(Cell(grid, 15, 1)); cur != nil {
		c.Exp = *cur
	}
	c.Gold = NumOrDefault(Cell(grid, 16, 1), 0)

	var weapons []model.Weapon
	for slot := 1; slot <= WeaponsPerCharacter; slot++ {
		row := slot + 1 // shift = 2 (row index starts at 2 for slot 1)
		name := Cell(grid, row, 3)
		damage := Cell(grid, row, 4)
		if name == "" || damage == "" {
			continue
		}
		nameCopy := name
		weapons = append(weapons, model.Weapon{
			UserID: userID,
			Number: slot,
			Name:   &nameCopy,
			Hit:    BuildHit(Cell(grid, row, 5)),
			Damage: damage,
			Crit:   ClampCrit(Cell(grid, row, 6)),
		})
	}
	return c, weapons
}

// ParseNpcSheet pulls an NPC's stats and its primary weapon (weaponNumber=1).
// The sheet layout is the same as characters, but weapon cells live in
// columns F/G/H (indices 5/6/7) instead of D/E/F/G.
func ParseNpcSheet(grid [][]string, sheetID, sheetName string, weaponNumber int) model.Npc {
	if weaponNumber < 1 {
		weaponNumber = 1
	}
	row := 1 + weaponNumber // shift=2 (row index 2 for weapon #1)

	n := model.Npc{
		Name:      Cell(grid, 0, 0),
		Level:     NumOrDefault(Cell(grid, 9, 1), 1),
		SheetID:   sheetID,
		SheetName: sheetName,
		IsAllowed: strings.TrimSpace(Cell(grid, 0, 2)) == "*",
		Hit:       BuildHit(Cell(grid, row, 6)),
		Crit:      ClampCrit(Cell(grid, row, 7)),
	}

	// Stat substitution: NPC damage formulas reference str/con/… like player
	// weapons, but the values live on the same sheet, so replace inline.
	dmg := Cell(grid, row, 5)
	if dmg == "" {
		dmg = "d8"
	}
	stats := map[string]string{
		"str": Cell(grid, 2, 1),
		"con": Cell(grid, 3, 1),
		"dex": Cell(grid, 4, 1),
		"int": Cell(grid, 5, 1),
		"wis": Cell(grid, 6, 1),
		"chr": Cell(grid, 7, 1),
	}
	lower := strings.ToLower(dmg)
	for token, value := range stats {
		if value != "" && strings.Contains(lower, token) {
			lower = strings.ReplaceAll(lower, token, value)
		}
	}
	n.Damage = lower

	if cur, max := ParseCurrentMax(Cell(grid, 10, 1)); max != nil {
		n.CurrentHP = cur
		n.MaxHP = max
	}
	if cur, max := ParseCurrentMax(Cell(grid, 11, 1)); max != nil {
		n.CurrentMP = cur
		n.MaxMP = max
	}
	return n
}

// SheetName is an empty fallback name so the bot never crashes on an empty
// first cell. Not used in sheetssync — /npc callers pass sheet_name directly.
