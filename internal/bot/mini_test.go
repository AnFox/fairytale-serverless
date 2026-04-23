package bot

import "testing"

func TestQualityForBoundaries(t *testing.T) {
	cases := []struct {
		roll int
		want string
	}{
		{1, "Обычный"}, {50, "Обычный"},
		{51, "Необычный"}, {75, "Необычный"},
		{76, "Магический"}, {90, "Магический"},
		{91, "Редкий"}, {97, "Редкий"},
		{98, "Эпический"}, {99, "Эпический"},
		{100, "Легендарный"},
	}
	for _, c := range cases {
		if got := qualityFor(c.roll).name; got != c.want {
			t.Errorf("qualityFor(%d) = %q, want %q", c.roll, got, c.want)
		}
	}
}

func TestSubtypesForKnownTypes(t *testing.T) {
	if len(subtypesFor("Оружие")) == 0 || len(subtypesFor("Зелье")) == 0 {
		t.Fatal("expected weapons and potions to have subtypes")
	}
	if subtypesFor("Артефакт") != nil {
		t.Fatal("expected no subtypes for Артефакт")
	}
}

func TestPlayerRostersAllConfigured(t *testing.T) {
	for _, k := range []string{"w", "wm", "w4", "w6", "w7"} {
		if len(playerRosters[k]) == 0 {
			t.Fatalf("roster %q is empty", k)
		}
	}
}
