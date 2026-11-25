package layout

import (
	"testing"
)

// firstImageRect returns the rectangle of the first image operation.
func firstImageRect(t *testing.T, doc *Document) (w, h int32) {
	t.Helper()
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind == ImageOpKind {
				return op.Rect.W, op.Rect.H
			}
		}
	}
	t.Fatal("no image operation")
	return 0, 0
}

// TestImageAttributeDimensionsMatchCss guards the review reproduction: the same
// image with HTML dimensions 10 by 10 and 200 by 200 must not produce identical
// geometry, and the attribute size must agree with the equivalent CSS size.
func TestImageAttributeDimensionsMatchCss(t *testing.T) {
	h := newHarness(t)
	writeCoverPNG(t, h.images.dir, "photo.png", 200, 100)
	cases := []struct {
		name string
		src  string
		w, h int32
	}{
		{"attribute 10", `<img src="photo.png" width="10" height="10">`, 10 * 750, 10 * 750},
		{"css 10", `<img src="photo.png" style="width:10px;height:10px">`, 10 * 750, 10 * 750},
		{"attribute 200", `<img src="photo.png" width="200" height="200">`, 200 * 750, 200 * 750},
		{"css 200", `<img src="photo.png" style="width:200px;height:200px">`, 200 * 750, 200 * 750},
		{"block attribute 10", `<img src="photo.png" width="10" height="10" style="display:block">`, 10 * 750, 10 * 750},
		{"block css 10", `<img src="photo.png" style="display:block;width:10px;height:10px">`, 10 * 750, 10 * 750},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := h.run(tc.src)
			w, ht := firstImageRect(t, doc)
			if w != tc.w || ht != tc.h {
				t.Fatalf("image = %dx%d, want %dx%d", w, ht, tc.w, tc.h)
			}
		})
	}
}

// TestImageAttributeAspectRatio guards the single-dimension rule: a declared
// width scales the intrinsic height by the same ratio.
func TestImageAttributeAspectRatio(t *testing.T) {
	h := newHarness(t)
	writeCoverPNG(t, h.images.dir, "photo.png", 200, 100)
	doc := h.run(`<img src="photo.png" width="50">`)
	w, ht := firstImageRect(t, doc)
	if w != 50*750 || ht != 25*750 {
		t.Fatalf("image = %dx%d, want 50x25 CSS pixels", w, ht)
	}
	// The same ratio from the height side.
	doc = h.run(`<img src="photo.png" height="25">`)
	w, ht = firstImageRect(t, doc)
	if w != 50*750 || ht != 25*750 {
		t.Fatalf("image = %dx%d, want 50x25 CSS pixels", w, ht)
	}
}

// TestInvalidImageAttributesAreAbsent guards the invalid-attribute rule: a
// negative, unit-bearing, percentage or absurd value leaves the intrinsic size,
// and zero is a legal non-visible dimension.
func TestInvalidImageAttributesAreAbsent(t *testing.T) {
	h := newHarness(t)
	writeCoverPNG(t, h.images.dir, "photo.png", 200, 100)
	for _, attrs := range []string{
		`width="-5" height="-5"`,
		`width="10px" height="50%"`,
		`width="999999999999999999999"`,
	} {
		doc := h.run(`<img src="photo.png" ` + attrs + `>`)
		w, ht := firstImageRect(t, doc)
		if w != 200*750 || ht != 100*750 {
			t.Fatalf("%s: image = %dx%d, want the intrinsic 200x100", attrs, w, ht)
		}
	}
	doc := h.run(`<img src="photo.png" width="0" height="0">`)
	w, ht := firstImageRect(t, doc)
	if w != 0 || ht != 0 {
		t.Fatalf("zero dimensions = %dx%d, want a non-visible 0x0", w, ht)
	}
}

// TestCssAutoOverridesImageAttributeAtLayout guards the precedence rule end to
// end: an explicit auto restores the intrinsic size.
func TestCssAutoOverridesImageAttributeAtLayout(t *testing.T) {
	h := newHarness(t)
	writeCoverPNG(t, h.images.dir, "photo.png", 200, 100)
	doc := h.run(`<img src="photo.png" width="10" height="10" style="width:auto;height:auto">`)
	w, ht := firstImageRect(t, doc)
	if w != 200*750 || ht != 100*750 {
		t.Fatalf("auto restored %dx%d, want the intrinsic 200x100", w, ht)
	}
}
