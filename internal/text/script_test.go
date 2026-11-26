package text

import (
	"errors"
	"reflect"
	"testing"
)

// TestJoiningScript guards the detector table: each script the engine refuses
// is recognised, and scripts that need no joining are not.
func TestJoiningScript(t *testing.T) {
	cases := []struct {
		name   string
		s      string
		script Script
		ok     bool
	}{
		{"Arabic", "مرحبا بالعالم", ScriptArabic, true},
		{"Arabic presentation form", "ﻢﻟﺎﻌﻟﺎﺑ ﺎﺒﺣﺮﻣ", ScriptArabic, true},
		{"Syriac", "ܫܠܡܐ", ScriptSyriac, true},
		{"N'Ko", "ߊߟߎ߫", ScriptNKo, true},
		{"Devanagari", "नमस्ते", ScriptIndic, true},
		{"Tamil", "வணக்கம்", ScriptIndic, true},
		{"Hebrew", "שלום", "", false},
		{"Cyrillic", "Привет", "", false},
		{"Greek", "Γειά", "", false},
		{"Latin", "hello", "", false},
		{"empty", "", "", false},
	}
	for _, c := range cases {
		script, ok := JoiningScript(c.s)
		if ok != c.ok || script != c.script {
			t.Errorf("%s: JoiningScript(%q) = %q, %v; want %q, %v", c.name, c.s, script, ok, c.script, c.ok)
		}
	}
}

// TestJoiningScripts guards the documented list: it names every script in the
// detector once, sorted.
func TestJoiningScripts(t *testing.T) {
	want := []Script{ScriptArabic, ScriptIndic, ScriptNKo, ScriptSyriac}
	if got := JoiningScripts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("JoiningScripts() = %v, want %v", got, want)
	}
}

// TestShapeRefusesJoiningScript guards that shaping refuses the script before
// it looks for a glyph, so a font that happens to carry isolated Arabic forms
// cannot make an unjoined document look like success.
func TestShapeRefusesJoiningScript(t *testing.T) {
	f := testFont(t)
	if _, err := Shape(f, 12000, "שלום"); err != nil {
		t.Fatalf("Hebrew must shape: %v", err)
	}
	_, err := Shape(f, 12000, "مرحبا")
	var scriptErr *UnsupportedScriptError
	if !errors.As(err, &scriptErr) {
		t.Fatalf("Shape(Arabic) error = %v, want *UnsupportedScriptError", err)
	}
	if scriptErr.Script != ScriptArabic {
		t.Fatalf("script = %q, want %q", scriptErr.Script, ScriptArabic)
	}
}
