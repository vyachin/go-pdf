package css

import "testing"

// computeImage returns the computed style of the first img of a document whose
// author stylesheet is cssText.
func computeImage(t *testing.T, attrs, cssText string) *Style {
	t.Helper()
	doc := mustDoc(t, "<style>"+cssText+"</style><img "+attrs+">")
	img := findElement(doc.Root, "img")
	if img == nil {
		t.Fatal("no img in the fixture")
	}
	author := Parse([]byte(cssText))
	return NewCascadeForDocument(DefaultStylesheet(), author).Compute(img, nil)
}

// TestImageDimensionsArePresentationHints guards the hint contract: a valid
// attribute becomes a CSS-pixel length, while author CSS overrides it.
func TestImageDimensionsArePresentationHints(t *testing.T) {
	st := computeImage(t, `width="10" height="20"`, "")
	if st.Width.Kind != MP || st.Width.Value != 10*cssPixelMP {
		t.Fatalf("width = %+v, want 10 CSS pixels", st.Width)
	}
	if st.Height.Kind != MP || st.Height.Value != 20*cssPixelMP {
		t.Fatalf("height = %+v, want 20 CSS pixels", st.Height)
	}
}

// TestCssOverridesImageHints guards precedence: author declarations, an explicit
// auto and !important all beat the attribute.
func TestCssOverridesImageHints(t *testing.T) {
	st := computeImage(t, `width="200" height="200"`, `img { width: 10px }`)
	if st.Width.Kind != MP || st.Width.Value != 10*cssPixelMP {
		t.Fatalf("author width = %+v, want 10 CSS pixels", st.Width)
	}
	if st.Height.Kind != MP || st.Height.Value != 200*cssPixelMP {
		t.Fatalf("the unoverridden height hint = %+v, want 200 CSS pixels", st.Height)
	}

	auto := computeImage(t, `width="200"`, `img { width: auto }`)
	if auto.Width.Kind != Auto {
		t.Fatalf("explicit auto lost to the attribute: %+v", auto.Width)
	}

	important := computeImage(t, `width="200"`, `img { width: 10px !important }`)
	if important.Width.Kind != MP || important.Width.Value != 10*cssPixelMP {
		t.Fatalf("!important lost to the attribute: %+v", important.Width)
	}

	inline := computeImage(t, `width="200" style="width: 12px"`, "")
	if inline.Width.Value != 12*cssPixelMP {
		t.Fatalf("inline width = %+v, want 12 CSS pixels", inline.Width)
	}
}

// TestInvalidImageHintsAreAbsent guards the parser: only a non-negative integer
// is a hint. Everything else — a sign, a unit, a percentage, an absurd value —
// leaves the property at its initial value instead of wrapping the geometry.
func TestInvalidImageHintsAreAbsent(t *testing.T) {
	for _, attrs := range []string{
		`width="-5"`,
		`width="10px"`,
		`width="50%"`,
		`width="1e3"`,
		`width=" 10 "`, // trimmed, so this one is valid and checked below
		`width="999999999999999999999"`,
		`height=""`,
	} {
		st := computeImage(t, attrs, "")
		if attrs == `width=" 10 "` {
			if st.Width.Kind != MP || st.Width.Value != 10*cssPixelMP {
				t.Fatalf("%s: whitespace around an integer is valid, got %+v", attrs, st.Width)
			}
			continue
		}
		if st.Width.Kind != Auto {
			t.Fatalf("%s: width = %+v, want the initial auto", attrs, st.Width)
		}
	}
}

// TestZeroImageHintIsValid guards zero: it is a legal hint meaning a
// non-visible dimension, not an absent one.
func TestZeroImageHintIsValid(t *testing.T) {
	st := computeImage(t, `width="0"`, "")
	if st.Width.Kind != MP || st.Width.Value != 0 {
		t.Fatalf("zero width = %+v, want an absolute zero", st.Width)
	}
}

// TestDimensionHintIsIgnoredOnOtherElements keeps the hint to images.
func TestDimensionHintIsIgnoredOnOtherElements(t *testing.T) {
	doc := mustDoc(t, `<p width="100">x</p>`)
	p := findElement(doc.Root, "p")
	if p == nil {
		t.Fatal("no p")
	}
	st := NewCascadeForDocument(DefaultStylesheet()).Compute(p, nil)
	if st.Width.Kind != Auto {
		t.Fatalf("a width attribute on p became a hint: %+v", st.Width)
	}
}
