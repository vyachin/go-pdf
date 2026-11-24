package layout

// coverRect returns the rectangle an image is painted into so that it covers
// rect: the image is scaled by the larger of the two factors and the overflow
// is split equally between opposite edges. A degenerate image or box returns
// the zero rectangle, which callers treat as "nothing to paint".
func coverRect(img *ImageData, rect Rect) Rect {
	if img == nil || img.Width <= 0 || img.Height <= 0 || rect.W <= 0 || rect.H <= 0 {
		return Rect{}
	}
	iw, ih := int64(img.Width), int64(img.Height)
	rw, rh := int64(rect.W), int64(rect.H)
	var w, h int64
	if rw*ih >= rh*iw {
		// The box is wider than the image ratio, so the width decides the
		// scale and the height follows.
		w = rw
		h = ih * rw / iw
	} else {
		h = rh
		w = iw * rh / ih
	}
	// Integer division may leave the covered rectangle a thousandth of a point
	// short of the box; the clip would then show a hairline of the page. Grow
	// instead of trusting the rounding.
	if w < rw {
		w = rw
	}
	if h < rh {
		h = rh
	}
	return Rect{
		X: rect.X + int32((rw-w)/2),
		Y: rect.Y + int32((rh-h)/2),
		W: int32(w),
		H: int32(h),
	}
}

// backgroundOp builds the image operation of a box background, or reports that
// there is nothing to paint. The clip is the box itself: a covered image is
// larger than its box on one axis by construction.
func backgroundOp(img *ImageData, rect Rect) (Op, bool) {
	covered := coverRect(img, rect)
	if covered.W <= 0 || covered.H <= 0 {
		return Op{}, false
	}
	clip := rect
	return Op{
		Kind:  ImageOpKind,
		Rect:  covered,
		Image: &ImageOp{Image: img, Clip: &clip},
	}, true
}
