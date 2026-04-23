package bot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/anfox/fairytale-serverless/internal/dice"
	"github.com/anfox/fairytale-serverless/internal/model"
)

// substituteAttrs replaces stat tokens in a damage formula with character values.
// Order matters: longer prefixes shouldn't be eaten by shorter ones (none here
// share a prefix, but keep loop order deterministic).
func substituteAttrs(formula string, c *model.Character) string {
	formula = strings.ToLower(formula)
	subs := []struct {
		token string
		val   int
	}{
		{"str", c.Str}, {"con", c.Con}, {"dex", c.Dex},
		{"int", c.Int}, {"wis", c.Wis}, {"chr", c.Chr},
	}
	for _, s := range subs {
		formula = strings.ReplaceAll(formula, s.token, strconv.Itoa(s.val))
	}
	return formula
}

// weaponOutcome is the result of resolving hit/damage against an optional AC.
// Mirrors the legacy hoplessHit / miss / crit branches in Handler.php.
type weaponOutcome struct {
	HitOutput   string  // multi-line; mutated like in PHP rollOut
	DamageRoll  *dice.Roll
	DamageBlock string // formatted "X x 2 = Y" if crit applied
}

func computeWeaponOutcome(hit dice.Roll, dmg dice.Roll, ac int) weaponOutcome {
	out := weaponOutcome{HitOutput: hit.Output}
	dmgPtr := &dmg

	hoplessHit := false
	maxHit := 20 + hit.Modifier
	if hit.Number == 20 && ac > maxHit {
		hoplessHit = true
		out.HitOutput += "\n🍀 Удачный удар!"
	} else {
		switch {
		case hit.Miss:
			out.HitOutput += "\n" + hit.CritLabel
			dmgPtr = nil
		case ac > 0 && hit.Sum < ac:
			out.HitOutput += "\n       Промах!"
			dmgPtr = nil
		}

		if hit.Crit && dmgPtr != nil {
			out.HitOutput += "\n" + hit.CritLabel
			doubled := dmgPtr.Sum * 2
			out.DamageBlock = fmt.Sprintf("%s x 2 = *%d*", dmgPtr.Output, doubled)
		}
	}

	if dmgPtr != nil {
		out.DamageRoll = dmgPtr
		if out.DamageBlock == "" {
			out.DamageBlock = dmgPtr.Output
		}
	}
	_ = hoplessHit
	return out
}

func formatDiceMessage(author string, r dice.Roll) string {
	var sb strings.Builder
	if author != "" {
		sb.WriteString("🧑 ")
		sb.WriteString(author)
		sb.WriteString(" ")
	}
	sb.WriteString(r.Input)
	sb.WriteString("\n🎲 ")
	sb.WriteString(r.Output)
	if r.CritLabel != "" {
		sb.WriteString("\n")
		sb.WriteString(r.CritLabel)
	}
	return sb.String()
}

func formatWeaponMessage(author string, w *model.Weapon, number, ac int, res weaponOutcome) string {
	name := ""
	if w.Name != nil {
		name = *w.Name
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "🧑 %s %s \\[%d]", author, name, number)
	fmt.Fprintf(&sb, "\n🎲 Попадание: %s", w.Hit)
	fmt.Fprintf(&sb, "\n       *%s*", res.HitOutput)
	if ac > 0 {
		fmt.Fprintf(&sb, "\n🛡 AC: %d", ac)
	}
	if res.DamageRoll != nil {
		fmt.Fprintf(&sb, "\n⚔️ Урон: %s\n       *%s*", w.Damage, res.DamageBlock)
	}
	return sb.String()
}
