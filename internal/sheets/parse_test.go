package sheets

import "testing"

func TestCell(t *testing.T) {
	g := [][]string{{"a", "b"}, {"c"}}
	if Cell(g, 0, 1) != "b" || Cell(g, 1, 5) != "" || Cell(g, 9, 0) != "" {
		t.Fatal("Cell: wrong defaults")
	}
}

func TestParseCurrentMaxRatio(t *testing.T) {
	cur, max := ParseCurrentMax("12/30")
	if cur == nil || *cur != 12 || max == nil || *max != 30 {
		t.Fatalf("12/30: got %v / %v", cur, max)
	}
}

func TestParseCurrentMaxSingle(t *testing.T) {
	cur, max := ParseCurrentMax("10")
	if cur == nil || *cur != 10 || max == nil || *max != 10 {
		t.Fatalf("10: got %v / %v", cur, max)
	}
}

func TestParseCurrentMaxEmpty(t *testing.T) {
	cur, max := ParseCurrentMax("")
	if cur != nil || max != nil {
		t.Fatalf("empty: expected nil,nil, got %v,%v", cur, max)
	}
}

func TestBuildHit(t *testing.T) {
	cases := map[string]string{
		"":   "d20",
		"0":  "d20",
		"+2": "d20+2",
		"3":  "d20+3",
		"-1": "d20-1",
	}
	for in, want := range cases {
		if got := BuildHit(in); got != want {
			t.Errorf("BuildHit(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClampCrit(t *testing.T) {
	if ClampCrit("") != 20 || ClampCrit("21") != 20 || ClampCrit("18") != 18 || ClampCrit("0") != 20 {
		t.Fatal("ClampCrit out of range")
	}
}
