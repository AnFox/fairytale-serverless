package bot

import (
	"strings"
	"testing"

	"github.com/anfox/fairytale-serverless/internal/dice"
	"github.com/anfox/fairytale-serverless/internal/model"
)

func TestSubstituteAttrs(t *testing.T) {
	c := &model.Character{Str: 4, Dex: 2, Con: 3}
	got := substituteAttrs("d8+str", c)
	if got != "d8+4" {
		t.Fatalf("expected d8+4, got %q", got)
	}
	got = substituteAttrs("D6+DEX-CON", c)
	if got != "d6+2-3" {
		t.Fatalf("expected d6+2-3, got %q", got)
	}
}

func TestLooksLikeFormulaAcceptsDiceForms(t *testing.T) {
	cases := []string{"d20", "2d6+3", "D20-1", "1d8"}
	for _, s := range cases {
		if !looksLikeFormula(s) {
			t.Errorf("%q: expected true", s)
		}
	}
}

func TestLooksLikeFormulaRejectsProse(t *testing.T) {
	cases := []string{"hello", "5", "", "what's d20?", "die"}
	for _, s := range cases {
		if looksLikeFormula(s) {
			t.Errorf("%q: expected false", s)
		}
	}
}

func TestIsInteger(t *testing.T) {
	if !isInteger("5") || isInteger("5a") || isInteger("") {
		t.Fatal("isInteger broken")
	}
}

func TestComputeWeaponOutcomeMissDropsDamage(t *testing.T) {
	hit := dice.Roll{Number: 1, Sum: 1, Output: "1", Miss: true, CritLabel: "🔴 Critical miss!"}
	dmg := dice.Roll{Sum: 5, Output: "5"}
	out := computeWeaponOutcome(hit, dmg, 0)
	if out.DamageRoll != nil {
		t.Fatal("miss must drop damage")
	}
	if !strings.Contains(out.HitOutput, "Critical miss") {
		t.Fatalf("expected miss label, got %q", out.HitOutput)
	}
}

func TestComputeWeaponOutcomeCritDoublesDamage(t *testing.T) {
	hit := dice.Roll{Number: 20, Sum: 20, Output: "20", Crit: true, CritLabel: "🟢 Critical hit!"}
	dmg := dice.Roll{Sum: 7, Output: "7"}
	out := computeWeaponOutcome(hit, dmg, 0)
	if out.DamageRoll == nil || !strings.Contains(out.DamageBlock, "x 2 = *14*") {
		t.Fatalf("expected doubled damage, got %+v", out)
	}
}

func TestComputeWeaponOutcomeACBelowSumIsMiss(t *testing.T) {
	hit := dice.Roll{Number: 5, Sum: 5, Output: "5"}
	dmg := dice.Roll{Sum: 3, Output: "3"}
	out := computeWeaponOutcome(hit, dmg, 10)
	if out.DamageRoll != nil {
		t.Fatal("AC > sum should drop damage")
	}
	if !strings.Contains(out.HitOutput, "Промах") {
		t.Fatalf("expected promah, got %q", out.HitOutput)
	}
}

func TestComputeWeaponOutcomeHopless20BeatsHighAC(t *testing.T) {
	// Nat 20 with AC bigger than 20+modifier still counts as hit.
	hit := dice.Roll{Number: 20, Sum: 22, Modifier: 2, Output: "20+2"}
	dmg := dice.Roll{Sum: 6, Output: "6"}
	out := computeWeaponOutcome(hit, dmg, 99)
	if out.DamageRoll == nil {
		t.Fatal("hopeless hit must keep damage")
	}
	if !strings.Contains(out.HitOutput, "Удачный удар") {
		t.Fatalf("expected hopeless-hit label, got %q", out.HitOutput)
	}
}
