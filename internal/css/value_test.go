package css

import "testing"

func TestOutOfRangeNumbersAreRejected(t *testing.T) {
	for _, src := range []string{"1e999pt", "1e100em", "1e999%", "-1e999px"} {
		if _, err := ParseLengthString(src); err == nil {
			t.Errorf("ParseLengthString(%q) accepted an out-of-range value", src)
		}
	}
	// Values that fit the stored range still parse.
	for _, src := range []string{"1000000pt", "20000em", "2000000%"} {
		if _, err := ParseLengthString(src); err != nil {
			t.Errorf("ParseLengthString(%q) failed: %v", src, err)
		}
	}
	if _, err := ParseLengthString("20001em"); err == nil {
		t.Error("an em length beyond the stored range was accepted")
	}
}

func TestPercentageResolveSaturatesInsteadOfWrapping(t *testing.T) {
	// 2,000,000% over a 200,000 mp basis is 4,000,000,000 mp, which does not
	// fit an int32 and used to become negative.
	got := Pct(2_000_000).Resolve(200_000)
	if got < 0 {
		t.Fatalf("Resolve returned %d, want a non-negative saturated length", got)
	}
	if got > maxMillipoints {
		t.Fatalf("Resolve returned %d, want at most %d", got, maxMillipoints)
	}
}

func TestLineHeightFactorRange(t *testing.T) {
	st := DefaultStyle()
	if err := ApplyDecl(st, "line-height", Tokenize([]byte("1e100")), 12000); err == nil {
		t.Fatal("an out-of-range line-height factor was accepted")
	}
	if err := ApplyDecl(st, "line-height", Tokenize([]byte("1e999")), 12000); err == nil {
		t.Fatal("an infinite line-height factor was accepted")
	}
	st = DefaultStyle()
	if err := ApplyDecl(st, "line-height", Tokenize([]byte("1.5")), 12000); err != nil {
		t.Fatalf("a normal line-height factor failed: %v", err)
	}
	// The used value saturates instead of narrowing.
	st.FontSize = maxMillipoints
	st.LineHeightFactor = 10
	if got := st.LineHeightMP(); got < 0 {
		t.Fatalf("LineHeightMP returned %d, want a non-negative value", got)
	}
}

func TestPropertiesRejectOutOfRangeNumbers(t *testing.T) {
	cases := []struct {
		property string
		value    string
	}{
		{"font-weight", "1e999"},
		{"font-size", "1e999pt"},
		{"width", "1e100em"},
		{"margin", "1e999px"},
		{"orphans", "1e999"},
		{"widows", "1e999"},
		{"background-color", "rgb(1e999, 0, 0)"},
	}
	for _, tc := range cases {
		st := DefaultStyle()
		if err := ApplyDecl(st, tc.property, Tokenize([]byte(tc.value)), 12000); err == nil {
			t.Errorf("%s: %s was accepted", tc.property, tc.value)
		}
	}
}
