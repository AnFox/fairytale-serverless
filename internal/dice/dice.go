// Package dice parses tabletop-style dice formulas (e.g. "2d6+3", "d20", "5")
// and produces rolls with crit/miss detection.
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

type Roll struct {
	Count    int
	Dice     int
	Modifier int
	Sign     int
	Input    string

	Number int   // first die result (used for crit)
	Rolls  []int // all individual dice results
	Sum    int   // sum of dice + signed modifier

	Crit bool
	Miss bool

	Output    string // e.g. "15+3" or "[3,5,6]-1"
	CritLabel string // " CRITICAL HIT!" / " Critical Miss!"
}

var formulaRE = regexp.MustCompile(`^(\d*)d(\d+)([+-]\d+)?$`)

// Parse turns a formula string into a Roll ready for Execute.
// Constant integers ("5") are represented as a zero-dice +5 modifier so they
// still round-trip through Execute with Sum == 5.
func Parse(input string) Roll {
	r := Roll{Input: strings.ReplaceAll(strings.ToLower(strings.TrimSpace(input)), " ", ""), Sign: 1}

	if n, err := strconv.Atoi(r.Input); err == nil {
		r.Count = 1
		r.Dice = 1
		r.Modifier = n
		return r
	}

	m := formulaRE.FindStringSubmatch(r.Input)
	if m == nil {
		// Unparseable → default to d20.
		r.Count, r.Dice = 1, 20
		return r
	}
	if m[1] == "" {
		r.Count = 1
	} else {
		r.Count, _ = strconv.Atoi(m[1])
	}
	r.Dice, _ = strconv.Atoi(m[2])
	if m[3] != "" {
		mod, _ := strconv.Atoi(m[3])
		r.Modifier = abs(mod)
		if mod < 0 {
			r.Sign = -1
		}
	}
	return r
}

// Execute performs the random draws and fills roll totals and output strings.
func (r Roll) Execute() Roll {
	return r.ExecuteWith(defaultRand{})
}

// ExecuteWith lets tests inject a deterministic source.
func (r Roll) ExecuteWith(src RandSource) Roll {
	r.Rolls = make([]int, 0, r.Count)
	sum := 0
	for i := 0; i < r.Count; i++ {
		// When Dice == 1 (constant-number path) we still record a 1 so Number
		// isn't zero; the modifier carries the real value.
		n := src.IntN(r.Dice) + 1
		r.Rolls = append(r.Rolls, n)
		sum += n
	}
	if len(r.Rolls) > 0 {
		r.Number = r.Rolls[0]
	}
	modVal := r.Modifier * r.Sign
	r.Sum = sum + modVal
	r.Output = formatOutput(r.Rolls, modVal)
	return r
}

// ApplyCrit sets Crit/Miss flags based on the first die. Crit/miss apply only
// to a single d20: d6 / d100 / 2d20 never produce a critical outcome.
func (r Roll) ApplyCrit(critThreshold int) Roll {
	if critThreshold <= 0 {
		critThreshold = 20
	}
	if r.Dice != 20 || r.Count != 1 {
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

// Roll is a convenience shortcut: Parse → Execute → ApplyCrit(20).
func RollFormula(input string) Roll {
	return Parse(input).Execute().ApplyCrit(20)
}

func formatOutput(rolls []int, modVal int) string {
	var body string
	if len(rolls) == 1 {
		body = strconv.Itoa(rolls[0])
	} else {
		parts := make([]string, len(rolls))
		for i, n := range rolls {
			parts[i] = strconv.Itoa(n)
		}
		body = "[" + strings.Join(parts, ",") + "]"
	}
	if modVal == 0 {
		return body
	}
	if modVal > 0 {
		return fmt.Sprintf("%s+%d", body, modVal)
	}
	return fmt.Sprintf("%s%d", body, modVal)
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
