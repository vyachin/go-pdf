package text

import (
	"fmt"
	"slices"
)

// Script names a writing system whose shaping needs contextual joining. The
// engine has no GSUB, so it cannot join the cursive scripts or reorder the
// Indic clusters; painting them would produce text that is wrong in a way a
// reader cannot miss.
type Script string

const (
	// ScriptArabic covers Arabic and the languages written with it.
	ScriptArabic Script = "Arabic"
	// ScriptSyriac covers Syriac.
	ScriptSyriac Script = "Syriac"
	// ScriptNKo covers N'Ko.
	ScriptNKo Script = "N'Ko"
	// ScriptIndic covers the Indic scripts, which need reordering and
	// conjunct substitution rather than cursive joining.
	ScriptIndic Script = "Indic"
)

// scriptRange is one contiguous run of code points that needs contextual
// shaping. The ranges are inclusive.
type scriptRange struct {
	lo, hi rune
	script Script
}

// joiningRanges is the detector table. It is the single source of truth the
// profile document is checked against, so a script cannot be added here
// without the documentation following it.
var joiningRanges = []scriptRange{
	{0x0600, 0x06ff, ScriptArabic}, // Arabic
	{0x0700, 0x074f, ScriptSyriac}, // Syriac
	{0x0750, 0x077f, ScriptArabic}, // Arabic Supplement
	{0x07c0, 0x07ff, ScriptNKo},    // N'Ko
	{0x08a0, 0x08ff, ScriptArabic}, // Arabic Extended-A
	{0x0900, 0x0dff, ScriptIndic},  // Devanagari through Sinhala
	{0xfb50, 0xfdff, ScriptArabic}, // Arabic Presentation Forms-A
	{0xfe70, 0xfeff, ScriptArabic}, // Arabic Presentation Forms-B
}

// JoiningScript returns the script of the first rune of s that needs
// contextual shaping the engine cannot perform, and whether there is one.
func JoiningScript(s string) (Script, bool) {
	for _, r := range s {
		// Every range starts above the Hebrew block, so the common scripts of
		// a business document skip the table lookup entirely.
		if r < 0x0600 {
			continue
		}
		for _, rng := range joiningRanges {
			if r >= rng.lo && r <= rng.hi {
				return rng.script, true
			}
		}
	}
	return "", false
}

// JoiningScripts returns the scripts the detector refuses, sorted by name. The
// profile document is checked against this list.
func JoiningScripts() []Script {
	out := make([]Script, 0, len(joiningRanges))
	for _, rng := range joiningRanges {
		if !slices.Contains(out, rng.script) {
			out = append(out, rng.script)
		}
	}
	slices.Sort(out)
	return out
}

// UnsupportedScriptError reports text whose script needs contextual shaping
// that the engine does not implement. It is an error rather than a warning
// because the alternative is a PDF whose text is silently wrong.
type UnsupportedScriptError struct {
	Script Script
}

func (e *UnsupportedScriptError) Error() string {
	return fmt.Sprintf("the %s script needs contextual joining, which the engine does not implement", e.Script)
}
