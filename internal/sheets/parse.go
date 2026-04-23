package sheets

import (
	"regexp"
	"strconv"
	"strings"
)

// Cell pulls grid[row][col] with safe defaults for ragged rows.
func Cell(grid [][]string, row, col int) string {
	if row < 0 || row >= len(grid) {
		return ""
	}
	r := grid[row]
	if col < 0 || col >= len(r) {
		return ""
	}
	return strings.TrimSpace(r[col])
}

// NumOrDefault parses the first signed-int substring (so "+5hp" → 5, "-3" →
// -3). Returns d if nothing numeric is present.
func NumOrDefault(s string, d int) int {
	if s == "" {
		return d
	}
	s = strings.TrimSpace(s)
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	m := numRE.FindString(s)
	if m == "" {
		return d
	}
	n, err := strconv.Atoi(m)
	if err != nil {
		return d
	}
	return n
}

var numRE = regexp.MustCompile(`-?\d+`)

// ParseCurrentMax turns "12/30" → (12, 30). Plain "10" → (10, 10). Missing/
// unparseable → (nil, nil). Used for HP and MP cells which can be either form.
func ParseCurrentMax(s string) (current, max *int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if idx := strings.Index(s, "/"); idx >= 0 {
		if n, ok := atoiOpt(s[:idx]); ok {
			current = &n
		}
		if n, ok := atoiOpt(s[idx+1:]); ok {
			max = &n
		}
		return
	}
	if n, ok := atoiOpt(s); ok {
		current = &n
		maxCopy := n
		max = &maxCopy
	}
	return
}

func atoiOpt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	m := numRE.FindString(s)
	if m == "" {
		return 0, false
	}
	n, err := strconv.Atoi(m)
	if err != nil {
		return 0, false
	}
	return n, true
}

// BuildHit turns a bonus cell like "+2" or "3" into "d20+2" / "d20+3".
// Empty or zero bonus → "d20". Negative bonus preserved.
func BuildHit(bonusCell string) string {
	if bonusCell == "" {
		return "d20"
	}
	n := NumOrDefault(bonusCell, 0)
	switch {
	case n > 0:
		return "d20+" + strconv.Itoa(n)
	case n < 0:
		return "d20" + strconv.Itoa(n)
	default:
		return "d20"
	}
}

// ClampCrit keeps crit thresholds within [1,20], defaulting to 20 if the cell
// is empty or out of range.
func ClampCrit(s string) int {
	n := NumOrDefault(s, 20)
	if n < 1 || n > 20 {
		return 20
	}
	return n
}
