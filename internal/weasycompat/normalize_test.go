package weasycompat

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "invoice", "invoice"},
		{"nfkc ligature", "ﬁle", "file"},
		{"nfkc fullwidth", "ＡＢＣ１２３", "ABC123"},
		{"non-breaking space", "a\u00a0b", "a b"},
		{"narrow no-break space", "a\u202fb", "a b"},
		{"soft hyphen", "hy\u00adphen", "hyphen"},
		{"zero-width space", "a\u200bb", "ab"},
		{"zero-width joiner", "a\u200db", "ab"},
		{"word joiner", "a\u2060b", "ab"},
		{"byte-order mark", "\ufeffinvoice", "invoice"},
		{"whitespace run", "a \n\t b", "a b"},
		{"case is kept", "Привет Мир", "Привет Мир"},
		{"punctuation is kept", "a,b! (c)", "a,b! (c)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalize(tt.in); got != tt.want {
				t.Fatalf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	for _, in := range []string{"ﬁle", "a\u00a0\u00adb", "ＡＢ", "Привет"} {
		once := normalize(in)
		if twice := normalize(once); twice != once {
			t.Fatalf("normalize is not idempotent for %q: %q then %q", in, once, twice)
		}
	}
}

func TestTokens(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"punctuation separates", "a,b! c.", []string{"a", "b", "c"}},
		{"digits are tokens", "1257797706706", []string{"1257797706706"}},
		{"letters and digits stay together", "DL31", []string{"DL31"}},
		{"case is kept", "Мир", []string{"Мир"}},
		{"duplicates are kept in order", "a b a", []string{"a", "b", "a"}},
		{"empty", "", nil},
		{"only punctuation", ".,;", nil},
		{"ligature split", "ﬁsh ﬂow", []string{"fish", "flow"}},
		{"symbol is not a token", "CDG ✈ LFLL", []string{"CDG", "LFLL"}},
		{"underscore is not a token", "a_b", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokens(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("tokens(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCountDiff(t *testing.T) {
	tests := []struct {
		name      string
		want, got map[string]int
		missing   []string
		extra     []string
	}{
		{
			name: "equal multisets",
			want: map[string]int{"a": 2, "b": 1},
			got:  map[string]int{"b": 1, "a": 2},
		},
		{
			name:    "one token lost",
			want:    map[string]int{"a": 1, "b": 1},
			got:     map[string]int{"a": 1},
			missing: []string{"b"},
		},
		{
			name:  "one token invented",
			want:  map[string]int{"a": 1},
			got:   map[string]int{"a": 1, "c": 1},
			extra: []string{"c"},
		},
		{
			name:    "a duplicate lost",
			want:    map[string]int{"a": 3},
			got:     map[string]int{"a": 1},
			missing: []string{"a x2"},
		},
		{
			name:  "a duplicate invented",
			want:  map[string]int{"a": 1},
			got:   map[string]int{"a": 3},
			extra: []string{"a x2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing, extra := countDiff(tt.want, tt.got)
			if !reflect.DeepEqual(missing, tt.missing) {
				t.Fatalf("missing = %q, want %q", missing, tt.missing)
			}
			if !reflect.DeepEqual(extra, tt.extra) {
				t.Fatalf("extra = %q, want %q", extra, tt.extra)
			}
		})
	}
}
