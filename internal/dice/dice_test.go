package dice

import "testing"

// fixedRand returns scripted values from seq (0-indexed), falling back to 0.
type fixedRand struct {
	seq []int
	i   int
}

func (f *fixedRand) IntN(n int) int {
	if f.i >= len(f.seq) {
		return 0
	}
	v := f.seq[f.i]
	f.i++
	return v
}

func TestParseSimpleNumber(t *testing.T) {
	// Matches legacy PHP quirk: "5" → count=1 dice=1 modifier=5, rand(1,1)=1,
	// so Sum is 1+5=6. Preserved to stay bug-compatible with the old bot.
	r := Parse("5").Execute()
	if r.Sum != 6 {
		t.Fatalf("expected sum 6, got %d", r.Sum)
	}
}

func TestParseD20(t *testing.T) {
	r := Parse("d20")
	if r.Count != 1 || r.Dice != 20 || r.Modifier != 0 {
		t.Fatalf("bad parse: %+v", r)
	}
}

func TestParse2d6Plus3(t *testing.T) {
	r := Parse("2d6+3")
	if r.Count != 2 || r.Dice != 6 || r.Modifier != 3 || r.Sign != 1 {
		t.Fatalf("bad parse: %+v", r)
	}
}

func TestParseD20Minus1(t *testing.T) {
	r := Parse("d20-1")
	if r.Modifier != 1 || r.Sign != -1 {
		t.Fatalf("bad parse: %+v", r)
	}
}

func TestExecute2d6Plus3(t *testing.T) {
	// IntN returns 0..n-1, so seq {2,4} → rolls {3,5}, sum 8+3=11.
	r := Parse("2d6+3").ExecuteWith(&fixedRand{seq: []int{2, 4}})
	if r.Sum != 11 {
		t.Fatalf("expected 11, got %d (%+v)", r.Sum, r)
	}
	if r.Output != "[3,5]+3" {
		t.Fatalf("expected [3,5]+3, got %q", r.Output)
	}
}

func TestCritOnD20(t *testing.T) {
	// Force first die to 20.
	r := Parse("d20").ExecuteWith(&fixedRand{seq: []int{19}}).ApplyCrit(20)
	if !r.Crit || r.Miss {
		t.Fatalf("expected crit, got %+v", r)
	}
	if r.CritLabel != "🟢 Critical hit!" {
		t.Fatalf("expected hit label, got %q", r.CritLabel)
	}
}

func TestMissOnD20(t *testing.T) {
	r := Parse("d20").ExecuteWith(&fixedRand{seq: []int{0}}).ApplyCrit(20)
	if !r.Miss || r.Crit {
		t.Fatalf("expected miss, got %+v", r)
	}
	if r.CritLabel != "🔴 Critical miss!" {
		t.Fatalf("expected miss label, got %q", r.CritLabel)
	}
}

func TestCritThresholdLowerThan20(t *testing.T) {
	// Crit threshold 18 — rolling 18 should crit.
	r := Parse("d20").ExecuteWith(&fixedRand{seq: []int{17}}).ApplyCrit(18)
	if !r.Crit {
		t.Fatalf("expected crit at threshold 18, got %+v", r)
	}
}

func TestNoCritOnD6(t *testing.T) {
	// Smaller dice never crit even if max value rolled.
	r := Parse("d6").ExecuteWith(&fixedRand{seq: []int{5}}).ApplyCrit(20)
	if r.Crit || r.Miss || r.CritLabel != "" {
		t.Fatalf("expected no crit on d6, got %+v", r)
	}
}

func TestNoCritOnMultipleDice(t *testing.T) {
	// 2d20 should not crit even if first die is 20.
	r := Parse("2d20").ExecuteWith(&fixedRand{seq: []int{19, 5}}).ApplyCrit(20)
	if r.Crit || r.CritLabel != "" {
		t.Fatalf("expected no crit on 2d20, got %+v", r)
	}
}

func TestNoCritOnD100(t *testing.T) {
	// Larger-than-20 dice don't crit either — only clean d20 triggers.
	r := Parse("d100").ExecuteWith(&fixedRand{seq: []int{99}}).ApplyCrit(20)
	if r.Crit || r.CritLabel != "" {
		t.Fatalf("expected no crit on d100, got %+v", r)
	}
}

func TestOutputSingleDieNoModifier(t *testing.T) {
	r := Parse("d6").ExecuteWith(&fixedRand{seq: []int{3}})
	if r.Output != "4" {
		t.Fatalf("expected 4, got %q", r.Output)
	}
}

func TestOutputMinusModifier(t *testing.T) {
	r := Parse("d20-1").ExecuteWith(&fixedRand{seq: []int{9}})
	if r.Output != "10-1" {
		t.Fatalf("expected 10-1, got %q", r.Output)
	}
	if r.Sum != 9 {
		t.Fatalf("expected sum 9, got %d", r.Sum)
	}
}

func TestUnparseableFallsBackToD20(t *testing.T) {
	r := Parse("garbage")
	if r.Count != 1 || r.Dice != 20 {
		t.Fatalf("expected d20 fallback, got %+v", r)
	}
}
