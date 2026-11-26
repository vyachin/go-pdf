package css

import "testing"

func TestLengthUnitsAndRounding(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want Length
	}{
		{"1pt", Abs(1000)}, {"1px", Abs(750)}, {"1in", Abs(72000)},
		{"25.4mm", Abs(72000)}, {"2.54cm", Abs(72000)},
		{"0.0005pt", Abs(1)}, {"-0.0005pt", Abs(-1)},
		{"1.25em", EmLen(1.25)}, {"12.5%", Pct(12.5)},
		{" \t1pt\n", Abs(1000)}, {"0", Abs(0)}, {"AUTO", Length{Kind: Auto}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got, err := ParseLengthString(tc.src)
			if err != nil || got != tc.want {
				t.Fatalf("ParseLengthString = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
	for _, src := range []string{"", "1", "1rem", "1vw", "1pt 2pt", "calc(1pt)", "red", "#fff"} {
		t.Run("invalid/"+src, func(t *testing.T) {
			if got, err := ParseLengthString(src); err == nil {
				t.Fatalf("accepted %q as %+v", src, got)
			}
		})
	}
}

func TestLengthResolutionContract(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value Length
		basis int32
		want  int32
	}{
		{"absolute ignores basis", Abs(1234), 99999, 1234},
		{"auto", Length{Kind: Auto}, 99999, 0},
		{"percentage", Pct(25), 12000, 3000},
		{"em", EmLen(1.5), 12000, 18000},
		{"fraction truncates", Pct(50), 1001, 500},
		{"negative percentage", Pct(-25), 12000, -3000},
		{"positive saturation", Pct(2_000_000), 200000, maxMillipoints},
		{"negative saturation", Pct(-2_000_000), 200000, -maxMillipoints},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.value.Resolve(tc.basis); got != tc.want {
				t.Fatalf("Resolve(%d) = %d, want %d", tc.basis, got, tc.want)
			}
		})
	}
	for _, length := range []Length{Abs(1234), Pct(25), {Kind: Auto}} {
		if got := length.ResolveFont(12000); got != length {
			t.Errorf("ResolveFont changed a non-em length: %+v -> %+v", length, got)
		}
	}
	if got := EmLen(1.5).ResolveFont(12000); got != Abs(18000) {
		t.Errorf("ResolveFont = %+v, want 18pt", got)
	}
}
