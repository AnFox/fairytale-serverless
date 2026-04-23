package sheets

import "testing"

// fixture mirrors the legacy A1:H17 layout — only the cells the parsers read.
func characterFixture() [][]string {
	return [][]string{
		/* row 0 */ {"label", "Venator"},
		/* 1 */ {"", ""},
		/* 2 str */ {"str", "3"},
		/* 3 con */ {"con", "2"},
		/* 4 dex */ {"dex", "5"},
		/* 5 int */ {"int", "1"},
		/* 6 wis */ {"wis", "2"},
		/* 7 chr */ {"chr", "0"},
		/* 8 */ {"", ""},
		/* 9 lvl */ {"lvl", "4"},
		/* 10 hp  */ {"hp", "20/30"},
		/* 11 mp  */ {"mp", "8/12"},
		/* 12 ac  */ {"ac", "16"},
		/* 13 arm */ {"armor", "2"},
		/* 14 pp  */ {"pp", "12"},
		/* 15 exp */ {"exp", "750"},
		/* 16 gld */ {"gold", "42"},
	}
	// Weapons rows are 2..6 (slots 1..5) cols 3..6.
	// Index above stops at 16 so weapons are absent in this fixture; tests
	// add them per case.
}

func TestParseCharacterStats(t *testing.T) {
	c, w := ParseCharacterSheet(characterFixture(), 7)
	if c.UserID != 7 || c.Name != "Venator" {
		t.Fatalf("user/name: %+v", c)
	}
	if c.Str != 3 || c.Dex != 5 || c.Wis != 2 {
		t.Fatalf("stats: %+v", c)
	}
	if c.Level != 4 || c.HP != 30 || c.MP != 12 || c.AC != 16 || c.Armor != 2 || c.PP != 12 || c.Exp != 750 || c.Gold != 42 {
		t.Fatalf("scalars: %+v", c)
	}
	if c.CurrentMP == nil || *c.CurrentMP != 8 || c.MaxMP == nil || *c.MaxMP != 12 {
		t.Fatalf("mp pair: %+v %+v", c.CurrentMP, c.MaxMP)
	}
	if len(w) != 0 {
		t.Fatalf("expected zero weapons, got %d", len(w))
	}
}

func TestParseCharacterWeaponsSkipBlankSlots(t *testing.T) {
	g := characterFixture()
	// Slot 1 (row 2): name+damage present.
	g[2] = []string{"str", "3", "", "Лук", "d8+DEX", "+2", "20"}
	// Slot 2 (row 3): blank → skip.
	g[3] = []string{"con", "2"}
	// Slot 3 (row 4): name only, damage missing → skip.
	g[4] = []string{"dex", "5", "", "Кинжал", ""}
	// Slot 4 (row 5): negative hit bonus, crit 18.
	g[5] = []string{"int", "1", "", "Магия", "d10+INT", "-1", "18"}

	_, w := ParseCharacterSheet(g, 7)
	if len(w) != 2 {
		t.Fatalf("expected 2 weapons, got %d (%+v)", len(w), w)
	}
	if w[0].Number != 1 || w[0].Hit != "d20+2" || w[0].Crit != 20 {
		t.Fatalf("slot 1 mismatch: %+v", w[0])
	}
	if w[1].Number != 4 || w[1].Hit != "d20-1" || w[1].Crit != 18 {
		t.Fatalf("slot 4 mismatch: %+v", w[1])
	}
}

func TestParseNpcSheetWithStatSubstitution(t *testing.T) {
	g := characterFixture()
	g[0] = []string{"Torvald", "", "*"} // is_allowed='*'
	// NPC primary weapon row = 2 (weaponNumber=1). Cols 5/6/7.
	g[2] = []string{"str", "3", "", "", "", "d10+STR+1", "+3", "19"}

	npc := ParseNpcSheet(g, "sheet-id-x", "torvald", 1)
	if npc.Name != "Torvald" || !npc.IsAllowed {
		t.Fatalf("name/allowed: %+v", npc)
	}
	if npc.Hit != "d20+3" {
		t.Fatalf("hit: %q", npc.Hit)
	}
	if npc.Damage != "d10+3+1" {
		t.Fatalf("damage substitution: %q", npc.Damage)
	}
	if npc.Crit != 19 {
		t.Fatalf("crit: %d", npc.Crit)
	}
}

func TestHashGridStable(t *testing.T) {
	g1 := characterFixture()
	g2 := characterFixture()
	if HashGrid(g1) != HashGrid(g2) {
		t.Fatal("hash should be stable for identical grids")
	}
	g2[0][1] = "Different"
	if HashGrid(g1) == HashGrid(g2) {
		t.Fatal("hash should differ when cell changes")
	}
}
