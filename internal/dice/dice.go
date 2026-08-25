// Package dice parses tabletop-style dice formulas (e.g. "2d6+3", "d20",
// "d10+3+1", "5") and produces rolls with crit/miss detection.
//
// Port of app/Services/DiceRollService.php from the legacy Laravel app.
// The first die result is used to detect crit/miss (crit threshold defaults to 20).
package dice

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
)

// Term is one additive component of a formula: either a dice group ("2d6")
// or a flat constant ("3"). Sign carries the leading +/-.
type Term struct {
	Count int // dice count; 0 for a constant term
	Sides int // die size; 0 for a constant term
	Value int // magnitude of a constant term
	Sign  int // +1 or -1
}

// IsDice reports whether the term rolls dice rather than adding a constant.
func (t Term) IsDice() bool {
	return t.Count > 0
}

type Roll struct {
	Count    int // dice count of the first dice group (crit logic + compat)
	Dice     int // die size of the first dice group
	Modifier int // magnitude of all constant terms combined
	Sign     int // sign of the combined constant terms
	Input    string

	Terms []Term // every component, in formula order

	Number int   // first die result (used for crit)
	Rolls  []int // all individual dice results
	Sum    int   // signed total of every term

	Crit bool
	Miss bool

	Output    string // e.g. "15+3", "[3,5]-1", "8+5+4"
	CritLabel string // "🟢 Critical hit!" / "🔴 Critical miss!"
}

var (
	// formulaRE validates a whole formula: dice/constant terms joined by +/-.
	// Sheet formulas routinely carry several modifiers ("d10+3+1" after stat
	// substitution), so a single trailing modifier is not enough.
	formulaRE = regexp.MustCompile(`^(?:\d*d\d+|\d+)(?:[+-](?:\d*d\d+|\d+))*$`)
	termRE    = regexp.MustCompile(`^(\d*)d(\d+)$`)
)

// Parse turns a formula string into a Roll ready for Execute.
// Constant integers ("5") are represented as a zero-dice +5 modifier so they
// still round-trip through Execute with Sum == 5.
func Parse(input string) Roll {
	r := Roll{Input: strings.ReplaceAll(strings.ToLower(strings.TrimSpace(input)), " ", ""), Sign: 1}

	if n, err := strconv.Atoi(r.Input); err == nil {
		r.Count = 1
		r.Dice = 1
		r.Modifier = n
		r.Terms = []Term{
			{Count: 1, Sides: 1, Sign: 1},
			{Value: n, Sign: 1},
		}
		return r
	}

	terms, ok := parseTerms(r.Input)
	if !ok {
		// Unparseable → default to d20.
		r.Count, r.Dice = 1, 20
		r.Terms = []Term{{Count: 1, Sides: 20, Sign: 1}}
		return r
	}
	r.Terms = terms

	constants := 0
	firstDice := true
	for _, t := range terms {
		if t.IsDice() {
			if firstDice {
				r.Count, r.Dice = t.Count, t.Sides
				firstDice = false
			}
			continue
		}
		constants += t.Value * t.Sign
	}
	r.Modifier = abs(constants)
	if constants < 0 {
		r.Sign = -1
	}
	return r
}

// parseTerms splits a normalized formula into its signed components.
func parseTerms(s string) ([]Term, bool) {
	if !formulaRE.MatchString(s) {
		return nil, false
	}
	var (
		terms []Term
		start int
		sign  = 1
	)
	flush := func(end int, nextSign int) bool {
		t, ok := parseTerm(s[start:end], sign)
		if !ok {
			return false
		}
		terms = append(terms, t)
		sign = nextSign
		start = end + 1
		return true
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '+':
			if !flush(i, 1) {
				return nil, false
			}
		case '-':
			if !flush(i, -1) {
				return nil, false
			}
		}
	}
	if !flush(len(s), 1) {
		return nil, false
	}
	return terms, true
}

func parseTerm(s string, sign int) (Term, bool) {
	if m := termRE.FindStringSubmatch(s); m != nil {
		count := 1
		if m[1] != "" {
			count, _ = strconv.Atoi(m[1])
		}
		sides, _ := strconv.Atoi(m[2])
		if count <= 0 || sides <= 0 {
			return Term{}, false
		}
		return Term{Count: count, Sides: sides, Sign: sign}, true
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return Term{}, false
	}
	return Term{Value: v, Sign: sign}, true
}

// Execute performs the random draws and fills roll totals and output strings.
func (r Roll) Execute() Roll {
	return r.ExecuteWith(defaultRand{})
}

// ExecuteWith lets tests inject a deterministic source.
func (r Roll) ExecuteWith(src RandSource) Roll {
	r.Rolls = make([]int, 0, len(r.Terms))
	sum := 0
	var out strings.Builder

	for i, t := range r.Terms {
		if i > 0 {
			if t.Sign < 0 {
				out.WriteByte('-')
			} else {
				out.WriteByte('+')
			}
		}
		if !t.IsDice() {
			sum += t.Value * t.Sign
			out.WriteString(strconv.Itoa(t.Value))
			continue
		}
		// When Sides == 1 (constant-number path) we still record a 1 so Number
		// isn't zero; the modifier carries the real value.
		group := make([]int, 0, t.Count)
		for j := 0; j < t.Count; j++ {
			n := src.IntN(t.Sides) + 1
			group = append(group, n)
			sum += n * t.Sign
		}
		r.Rolls = append(r.Rolls, group...)
		out.WriteString(formatGroup(group))
	}

	if len(r.Rolls) > 0 {
		r.Number = r.Rolls[0]
	}
	r.Sum = sum
	r.Output = out.String()
	return r
}

// ApplyCrit sets Crit/Miss flags based on the first die. Crit/miss apply only
// to a single d20: d6 / d100 / 2d20 never produce a critical outcome.
func (r Roll) ApplyCrit(critThreshold int) Roll {
	if critThreshold <= 0 {
		critThreshold = 20
	}
	if r.Dice != 20 || r.Count != 1 || countDiceTerms(r.Terms) > 1 {
		return r
	}
	if r.Number == 1 {
		r.Miss = true
		r.CritLabel = "🔴 Critical miss!"
	} else if r.Number >= critThreshold {
		r.Crit = true
		r.CritLabel = "🟢 Critical hit!"
	}
	return r
}

// Display renders the roll for chat: the expression plus its total, e.g.
// "3+5+4 = 12". A bare single die already reads as its own total, so "8"
// stays "8" instead of the noisier "8 = 8".
func (r Roll) Display() string {
	if r.Output == strconv.Itoa(r.Sum) {
		return r.Output
	}
	return fmt.Sprintf("%s = %d", r.Output, r.Sum)
}

// Roll is a convenience shortcut: Parse → Execute → ApplyCrit(20).
func RollFormula(input string) Roll {
	return Parse(input).Execute().ApplyCrit(20)
}

func countDiceTerms(terms []Term) int {
	n := 0
	for _, t := range terms {
		if t.IsDice() {
			n++
		}
	}
	return n
}

func formatGroup(rolls []int) string {
	if len(rolls) == 1 {
		return strconv.Itoa(rolls[0])
	}
	parts := make([]string, len(rolls))
	for i, n := range rolls {
		parts[i] = strconv.Itoa(n)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// RandSource abstracts randomness so Execute can be tested deterministically.
type RandSource interface {
	IntN(n int) int
}

type defaultRand struct{}

func (defaultRand) IntN(n int) int {
	if n <= 1 {
		return 0
	}
	return rand.IntN(n)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
