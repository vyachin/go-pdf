package font

import (
	"encoding/binary"
	"fmt"
)

// cffOp is the kind of one outline segment.
type cffOp uint8

const (
	cffMoveTo cffOp = iota
	cffLineTo
	cffCubeTo
)

// cffPoint is one point of a CFF outline in 26.6 font units, the y axis
// pointing up.
type cffPoint struct{ X, Y int32 }

// cffSegment is one segment of a CFF outline: a move, a line or a cubic curve.
// A cubic carries its two control points and its end point.
type cffSegment struct {
	Op   cffOp
	Args [3]cffPoint
}

// The limits of the Type 2 virtual machine.
const (
	// cffArgStackSize is the 48 operand positions the format allows.
	cffArgStackSize = 48
	// cffTransientSize is the 32 slots of the transient array of put and get.
	cffTransientSize = 32
	// cffMaxInstructions bounds the work of one glyph across its subroutines.
	// A charstring that recurses is already stopped at cffMaxSubrDepth; this
	// stops a long but legal one from drawing for minutes.
	cffMaxInstructions = 100_000
	// cffMaxTotalInstructions bounds the work of the whole font, because the
	// per-glyph bound multiplies by the declared glyph count: 65,535 glyphs of
	// 100,000 instructions each is 6.5 billion instructions from one file. The
	// cap is a backstop for a caller with no work guard; the guard normally
	// stops the conversion first.
	cffMaxTotalInstructions = 12_000_000
	// cffTrue is the fixed-point value of "true" for the arithmetic operators.
	cffTrue = int64(1) << 16
	// cffFixedOne is one font unit in 16.16 fixed point.
	cffFixedOne = int64(1) << 16
)

// The two-byte (escaped) Type 2 operators this reader implements.
const (
	cffOpDotsection = 0x0c00
	cffOpAnd        = 0x0c03
	cffOpOr         = 0x0c04
	cffOpNot        = 0x0c05
	cffOpAbs        = 0x0c09
	cffOpAdd        = 0x0c0a
	cffOpSub        = 0x0c0b
	cffOpDiv        = 0x0c0c
	cffOpNeg        = 0x0c0e
	cffOpEq         = 0x0c0f
	cffOpDrop       = 0x0c12
	cffOpPut        = 0x0c14
	cffOpGet        = 0x0c15
	cffOpIfelse     = 0x0c16
	cffOpRandom     = 0x0c17
	cffOpMul        = 0x0c18
	cffOpSqrt       = 0x0c1a
	cffOpDup        = 0x0c1b
	cffOpExch       = 0x0c1c
	cffOpIndex      = 0x0c1d
	cffOpRoll       = 0x0c1e
	cffOpHFlex      = 0x0c22
	cffOpFlex       = 0x0c23
	cffOpHFlex1     = 0x0c24
	cffOpFlex1      = 0x0c25
)

// cffInterp draws one Type 2 charstring.
type cffInterp struct {
	font *cffTable
	fd   *cffFontDict
	gid  uint16
	// x and y accumulate in 16.16 font units. Keeping the fraction until a
	// segment is emitted means a chain of a hundred deltas does not drift by a
	// rounding error per operand, which matters at a quarter-unit tolerance.
	x, y         int64
	stack        []int64
	hints        int
	seenWidth    bool
	transient    [cffTransientSize]int64
	segments     []cffSegment
	instructions int
	points       int
	ended        bool
}

// glyphOutline returns the outline segments of one glyph.
func (t *cffTable) glyphOutline(gid uint16) ([]cffSegment, error) {
	if int(gid) >= len(t.charStrings) {
		return nil, fmt.Errorf("font: CFF has no charstring for glyph %d", gid)
	}
	fd := &t.fds[0]
	if t.fdSelect != nil {
		fd = &t.fds[t.fdSelect[gid]]
	}
	in := &cffInterp{font: t, fd: fd, gid: gid}
	if err := in.run(t.charStrings[gid], 0); err != nil {
		return nil, err
	}
	if !in.ended {
		return nil, fmt.Errorf("font: CFF glyph %d does not end with endchar", gid)
	}
	return in.segments, nil
}

// run executes one charstring or subroutine. depth is the subroutine nesting,
// zero for the glyph's own charstring.
func (t *cffInterp) run(code []byte, depth int) error {
	for pos := 0; pos < len(code); {
		t.instructions++
		if t.instructions > cffMaxInstructions {
			return fmt.Errorf("%w: glyph %d runs more than %d charstring instructions", ErrLimit, t.gid, cffMaxInstructions)
		}
		// Every instruction is charged, so the conversion is bounded by the
		// same budget as the rest of the font's parsing.
		t.font.instructions++
		if t.font.instructions > cffMaxTotalInstructions {
			return fmt.Errorf("%w: the CFF conversion runs more than %d charstring instructions", ErrLimit, cffMaxTotalInstructions)
		}
		if err := cffCharge(t.font.guard, 1); err != nil {
			return err
		}
		b := code[pos]
		if b == 28 || b >= 32 {
			value, next, err := cffCharstringNumber(code, pos)
			if err != nil {
				return fmt.Errorf("font: CFF glyph %d: %w", t.gid, err)
			}
			if err := t.push(value); err != nil {
				return err
			}
			pos = next
			continue
		}
		op := uint16(b)
		pos++
		if b == 12 {
			if pos >= len(code) {
				return fmt.Errorf("font: CFF glyph %d ends inside a two-byte operator", t.gid)
			}
			op = 0x0c00 | uint16(code[pos])
			pos++
		}
		next, done, err := t.operator(op, code, pos, depth)
		if err != nil {
			return err
		}
		pos = next
		// endchar ends the glyph, not just the subroutine that contains it.
		if done || t.ended {
			return nil
		}
	}
	if depth > 0 {
		return fmt.Errorf("font: CFF glyph %d calls a subroutine that ends without return", t.gid)
	}
	return nil
}

// operator executes one operator and returns the offset just past its data and
// whether this level of execution stops.
func (t *cffInterp) operator(op uint16, code []byte, pos, depth int) (next int, done bool, err error) {
	switch op {
	case 1, 3, 18, 23: // hstem, vstem, hstemhm, vstemhm
		return pos, false, t.stem()
	case 4: // vmoveto
		return pos, false, t.vmoveTo()
	case 5: // rlineto
		return pos, false, t.rlineTo()
	case 6: // hlineto
		return pos, false, t.lineToOp(false)
	case 7: // vlineto
		return pos, false, t.lineToOp(true)
	case 8: // rrcurveto
		return pos, false, t.rrcurveto()
	case 10: // callsubr
		return pos, false, t.callSubr(depth, false)
	case 11: // return
		if depth == 0 {
			return pos, false, fmt.Errorf("font: CFF glyph %d returns from its own charstring", t.gid)
		}
		return pos, true, nil
	case 14: // endchar
		return pos, true, t.endChar()
	case 19, 20: // hintmask, cntrmask
		after, err := t.mask(code, pos)
		return after, false, err
	case 21: // rmoveto
		return pos, false, t.rmoveTo()
	case 22: // hmoveto
		return pos, false, t.hmoveTo()
	case 24: // rcurveline
		return pos, false, t.rcurveline()
	case 25: // rlinecurve
		return pos, false, t.rlinecurve()
	case 26: // vvcurveto
		return pos, false, t.curvetoOp(false, true)
	case 27: // hhcurveto
		return pos, false, t.curvetoOp(false, false)
	case 29: // callgsubr
		return pos, false, t.callSubr(depth, true)
	case 30: // vhcurveto
		return pos, false, t.curvetoOp(true, true)
	case 31: // hvcurveto
		return pos, false, t.curvetoOp(true, false)
	case cffOpHFlex:
		return pos, false, t.hflex()
	case cffOpFlex:
		return pos, false, t.flex()
	case cffOpHFlex1:
		return pos, false, t.hflex1()
	case cffOpFlex1:
		return pos, false, t.flex1()
	}
	return pos, false, t.arithmetic(op)
}

// cffCharstringNumber reads one number operand and returns it in 16.16 font
// units.
func cffCharstringNumber(code []byte, pos int) (int64, int, error) {
	b := code[pos]
	switch {
	case b == 28:
		if pos+3 > len(code) {
			return 0, 0, fmt.Errorf("the charstring ends inside a number")
		}
		return int64(int16(binary.BigEndian.Uint16(code[pos+1:]))) * cffFixedOne, pos + 3, nil
	case b <= 246:
		return (int64(b) - 139) * cffFixedOne, pos + 1, nil
	case b <= 250:
		if pos+2 > len(code) {
			return 0, 0, fmt.Errorf("the charstring ends inside a number")
		}
		return (int64(b-247)*256 + int64(code[pos+1]) + 108) * cffFixedOne, pos + 2, nil
	case b <= 254:
		if pos+2 > len(code) {
			return 0, 0, fmt.Errorf("the charstring ends inside a number")
		}
		return (-(int64(b-251)*256 + int64(code[pos+1]) + 108)) * cffFixedOne, pos + 2, nil
	default: // 255: a signed 16.16 fixed-point number.
		if pos+5 > len(code) {
			return 0, 0, fmt.Errorf("the charstring ends inside a number")
		}
		return int64(int32(binary.BigEndian.Uint32(code[pos+1:]))), pos + 5, nil
	}
}

// stem consumes the operands of a stem hint. Hints are not applied: the stems
// are counted only because their number decides how many bytes a hint mask
// carries.
func (t *cffInterp) stem() error {
	t.readWidth(-1)
	if len(t.stack)%2 != 0 {
		return errCFFOperands(t.gid, "stem hint")
	}
	t.hints += len(t.stack) / 2
	if t.hints > cffMaxHints {
		return fmt.Errorf("font: CFF glyph %d declares %d stem hints, the limit is %d", t.gid, t.hints, cffMaxHints)
	}
	t.stack = t.stack[:0]
	return nil
}

// readWidth consumes the optional width operand in front of the first
// stack-clearing operator. The width is ignored: an OpenType font keeps its
// advances in hmtx, and the converted font copies that table.
func (t *cffInterp) readWidth(nArgs int) {
	if t.seenWidth {
		return
	}
	t.seenWidth = true
	if nArgs >= 0 {
		if len(t.stack) != nArgs+1 {
			return
		}
	} else if len(t.stack)%2 == 0 {
		return
	}
	t.stack = t.stack[1:]
}

func (t *cffInterp) rmoveTo() error {
	t.readWidth(2)
	if len(t.stack) != 2 {
		return errCFFOperands(t.gid, "rmoveto")
	}
	defer t.clear()
	return t.moveTo(t.stack[0], t.stack[1])
}

func (t *cffInterp) hmoveTo() error {
	t.readWidth(1)
	if len(t.stack) != 1 {
		return errCFFOperands(t.gid, "hmoveto")
	}
	defer t.clear()
	return t.moveTo(t.stack[0], 0)
}

func (t *cffInterp) vmoveTo() error {
	t.readWidth(1)
	if len(t.stack) != 1 {
		return errCFFOperands(t.gid, "vmoveto")
	}
	defer t.clear()
	return t.moveTo(0, t.stack[0])
}

func (t *cffInterp) moveTo(dx, dy int64) error {
	t.x += dx
	t.y += dy
	return t.emit(cffMoveTo, t.point(t.x, t.y))
}

func (t *cffInterp) rlineTo() error {
	if len(t.stack) < 2 || len(t.stack)%2 != 0 {
		return errCFFOperands(t.gid, "rlineto")
	}
	for i := 0; i < len(t.stack); i += 2 {
		if err := t.lineTo(t.stack[i], t.stack[i+1]); err != nil {
			return err
		}
	}
	t.stack = t.stack[:0]
	return nil
}

// lineToOp implements hlineto and vlineto, which alternate the axis of every
// operand.
func (t *cffInterp) lineToOp(vertical bool) error {
	if len(t.stack) < 1 {
		return errCFFOperands(t.gid, "lineto")
	}
	for i := 0; i < len(t.stack); i, vertical = i+1, !vertical {
		dx, dy := t.stack[i], int64(0)
		if vertical {
			dx, dy = dy, dx
		}
		if err := t.lineTo(dx, dy); err != nil {
			return err
		}
	}
	t.stack = t.stack[:0]
	return nil
}

func (t *cffInterp) lineTo(dx, dy int64) error {
	t.x += dx
	t.y += dy
	return t.emit(cffLineTo, t.point(t.x, t.y))
}

func (t *cffInterp) rrcurveto() error {
	if len(t.stack) < 6 || len(t.stack)%6 != 0 {
		return errCFFOperands(t.gid, "rrcurveto")
	}
	for i := 0; i < len(t.stack); i += 6 {
		if err := t.cubeTo(t.stack[i], t.stack[i+1], t.stack[i+2], t.stack[i+3], t.stack[i+4], t.stack[i+5]); err != nil {
			return err
		}
	}
	t.stack = t.stack[:0]
	return nil
}

// curvetoOp implements the hh, vv, hv and vh curve operators. Every curve of
// the group has one implied zero delta; for hv and vh the axis of the implied
// delta alternates, and the group may carry one extra operand for the last
// curve's other delta.
func (t *cffInterp) curvetoOp(swap, vertical bool) error {
	if len(t.stack) < 4 {
		return errCFFOperands(t.gid, "curveto")
	}
	i := 0
	switch len(t.stack) & 3 {
	case 0:
		// No leading delta.
	case 1:
		if swap {
			// The extra operand belongs to the last curve and is consumed
			// there.
			break
		}
		// The optional operand is the first curve's delta on the other axis:
		// dy1 for hhcurveto and dx1 for vvcurveto.
		i = 1
		if vertical {
			t.x += t.stack[0]
		} else {
			t.y += t.stack[0]
		}
	default:
		return errCFFOperands(t.gid, "curveto")
	}
	for i < len(t.stack) {
		next, err := t.curveto4(swap, vertical, i)
		if err != nil {
			return err
		}
		i = next
		if swap {
			vertical = !vertical
		}
	}
	t.stack = t.stack[:0]
	return nil
}

// curveto4 draws one curve of the hh/vv/hv/vh family starting at operand i and
// returns the index of the next one.
func (t *cffInterp) curveto4(swap, vertical bool, i int) (int, error) {
	if i+4 > len(t.stack) {
		return 0, errCFFOperands(t.gid, "curveto")
	}
	dxa, dya := t.stack[i], int64(0)
	dxb, dyb := t.stack[i+1], t.stack[i+2]
	dxc, dyc := t.stack[i+3], int64(0)
	i += 4
	if vertical {
		dxa, dya = dya, dxa
	}
	if swap && i+1 == len(t.stack) {
		dyc = t.stack[i]
		i++
	}
	if swap != vertical {
		dxc, dyc = dyc, dxc
	}
	return i, t.cubeTo(dxa, dya, dxb, dyb, dxc, dyc)
}

func (t *cffInterp) rcurveline() error {
	if len(t.stack) < 8 || len(t.stack)%6 != 2 {
		return errCFFOperands(t.gid, "rcurveline")
	}
	i := 0
	for iMax := len(t.stack) - 2; i < iMax; i += 6 {
		if err := t.cubeTo(t.stack[i], t.stack[i+1], t.stack[i+2], t.stack[i+3], t.stack[i+4], t.stack[i+5]); err != nil {
			return err
		}
	}
	if err := t.lineTo(t.stack[i], t.stack[i+1]); err != nil {
		return err
	}
	t.stack = t.stack[:0]
	return nil
}

func (t *cffInterp) rlinecurve() error {
	if len(t.stack) < 8 || len(t.stack)%2 != 0 {
		return errCFFOperands(t.gid, "rlinecurve")
	}
	i := 0
	for iMax := len(t.stack) - 6; i < iMax; i += 2 {
		if err := t.lineTo(t.stack[i], t.stack[i+1]); err != nil {
			return err
		}
	}
	if err := t.cubeTo(t.stack[i], t.stack[i+1], t.stack[i+2], t.stack[i+3], t.stack[i+4], t.stack[i+5]); err != nil {
		return err
	}
	t.stack = t.stack[:0]
	return nil
}

// flex draws the two curves of the flex family, whose second curve is closed by
// a computed delta.
func (t *cffInterp) flex() error {
	if len(t.stack) != 13 {
		return errCFFOperands(t.gid, "flex")
	}
	a := t.stack
	if err := t.cubeTo(a[0], a[1], a[2], a[3], a[4], a[5]); err != nil {
		return err
	}
	if err := t.cubeTo(a[6], a[7], a[8], a[9], a[10], a[11]); err != nil {
		return err
	}
	t.stack = t.stack[:0]
	return nil
}

func (t *cffInterp) hflex() error {
	if len(t.stack) != 7 {
		return errCFFOperands(t.gid, "hflex")
	}
	a := t.stack
	if err := t.cubeTo(a[0], 0, a[1], a[2], a[3], 0); err != nil {
		return err
	}
	// The second curve mirrors the first one's vertical delta.
	if err := t.cubeTo(a[4], 0, a[5], -a[2], a[6], 0); err != nil {
		return err
	}
	t.stack = t.stack[:0]
	return nil
}

func (t *cffInterp) hflex1() error {
	if len(t.stack) != 9 {
		return errCFFOperands(t.gid, "hflex1")
	}
	a := t.stack
	dy6 := -a[1] - a[3] - a[7]
	if err := t.cubeTo(a[0], a[1], a[2], a[3], a[4], 0); err != nil {
		return err
	}
	if err := t.cubeTo(a[5], 0, a[6], a[7], a[8], dy6); err != nil {
		return err
	}
	t.stack = t.stack[:0]
	return nil
}

func (t *cffInterp) flex1() error {
	if len(t.stack) != 11 {
		return errCFFOperands(t.gid, "flex1")
	}
	a := t.stack
	// The last operand closes the flex along the axis with less travel: the
	// other final delta is the negation of the five deltas on that axis.
	horizontal := cffAbs(a[0]) + cffAbs(a[2]) + cffAbs(a[4]) + cffAbs(a[6]) + cffAbs(a[8])
	vertical := cffAbs(a[1]) + cffAbs(a[3]) + cffAbs(a[5]) + cffAbs(a[7]) + cffAbs(a[9])
	var dx6, dy6 int64
	if horizontal > vertical {
		dx6 = a[10]
		dy6 = -(a[1] + a[3] + a[5] + a[7] + a[9])
	} else {
		dy6 = a[10]
		dx6 = -(a[0] + a[2] + a[4] + a[6] + a[8])
	}
	if err := t.cubeTo(a[0], a[1], a[2], a[3], a[4], a[5]); err != nil {
		return err
	}
	if err := t.cubeTo(a[6], a[7], a[8], a[9], dx6, dy6); err != nil {
		return err
	}
	t.stack = t.stack[:0]
	return nil
}

func (t *cffInterp) cubeTo(dxa, dya, dxb, dyb, dxc, dyc int64) error {
	t.x += dxa
	t.y += dya
	p1 := t.point(t.x, t.y)
	t.x += dxb
	t.y += dyb
	p2 := t.point(t.x, t.y)
	t.x += dxc
	t.y += dyc
	p3 := t.point(t.x, t.y)
	return t.emit(cffCubeTo, p1, p2, p3)
}

// mask consumes the data of a hint or counter mask. A mask may stand in for the
// vstem operator, in which case the operands on the stack are its stems.
func (t *cffInterp) mask(code []byte, pos int) (int, error) {
	if len(t.stack) != 0 {
		if err := t.stem(); err != nil {
			return 0, err
		}
	} else if !t.seenWidth {
		// A mask is the first stack-clearing operator, so a lone operand in
		// front of it is the width.
		t.seenWidth = true
	}
	n := (t.hints + 7) / 8
	if pos+n > len(code) {
		return 0, fmt.Errorf("font: CFF glyph %d has a truncated hint mask", t.gid)
	}
	return pos + n, nil
}

func (t *cffInterp) callSubr(depth int, global bool) error {
	subrs := t.fd.subrs
	if global {
		subrs = t.font.gsubrs
	}
	if len(t.stack) == 0 {
		return fmt.Errorf("font: CFF glyph %d calls a subroutine with an empty stack", t.gid)
	}
	number := t.pop() / cffFixedOne
	if depth+1 > cffMaxSubrDepth {
		return fmt.Errorf("font: CFF glyph %d nests subroutines deeper than %d", t.gid, cffMaxSubrDepth)
	}
	index := int(number) + cffSubrBias(len(subrs))
	if index < 0 || index >= len(subrs) {
		return fmt.Errorf("font: CFF glyph %d calls subroutine %d of %d", t.gid, index, len(subrs))
	}
	return t.run(subrs[index], depth+1)
}

// cffSubrBias is the offset Type 2 adds to a subroutine number.
func cffSubrBias(n int) int {
	switch {
	case n < 1240:
		return 107
	case n < 33900:
		return 1131
	default:
		return 32768
	}
}

func (t *cffInterp) endChar() error {
	t.readWidth(0)
	if len(t.stack) == 4 {
		// 5177.Type2.pdf Appendix C: the deprecated accent composition of
		// Type 1. Composing it needs the charset and StandardEncoding, which
		// this reader does not keep; a diagnostic is better than a glyph with
		// its accent missing.
		return fmt.Errorf("font: CFF glyph %d uses the deprecated seac form of endchar", t.gid)
	}
	if len(t.stack) != 0 {
		return errCFFOperands(t.gid, "endchar")
	}
	t.stack = t.stack[:0]
	t.ended = true
	return nil
}

// arithmetic executes the stack and arithmetic operators. They occur in hint
// programs, which are parsed but not applied; they still have to execute
// because they decide what the path operators see.
func (t *cffInterp) arithmetic(op uint16) error {
	switch op {
	case cffOpDotsection:
		// Deprecated in Type 2 and without effect.
		return nil
	case cffOpAnd, cffOpOr:
		b, err := t.pop1()
		if err != nil {
			return err
		}
		a, err := t.pop1()
		if err != nil {
			return err
		}
		result := a != 0 && b != 0
		if op == cffOpOr {
			result = a != 0 || b != 0
		}
		return t.push(cffBool(result))
	case cffOpNot:
		a, err := t.pop1()
		if err != nil {
			return err
		}
		return t.push(cffBool(a == 0))
	case cffOpAbs:
		a, err := t.pop1()
		if err != nil {
			return err
		}
		return t.push(cffAbs(a))
	case cffOpAdd, cffOpSub:
		b, err := t.pop1()
		if err != nil {
			return err
		}
		a, err := t.pop1()
		if err != nil {
			return err
		}
		if op == cffOpSub {
			return t.push(a - b)
		}
		return t.push(a + b)
	case cffOpDiv:
		b, err := t.pop1()
		if err != nil {
			return err
		}
		a, err := t.pop1()
		if err != nil {
			return err
		}
		if b == 0 {
			return fmt.Errorf("font: CFF glyph %d divides by zero", t.gid)
		}
		return t.push(a * cffFixedOne / b)
	case cffOpNeg:
		a, err := t.pop1()
		if err != nil {
			return err
		}
		return t.push(-a)
	case cffOpEq:
		b, err := t.pop1()
		if err != nil {
			return err
		}
		a, err := t.pop1()
		if err != nil {
			return err
		}
		return t.push(cffBool(a == b))
	case cffOpDrop:
		_, err := t.pop1()
		return err
	case cffOpPut:
		index, err := t.pop1()
		if err != nil {
			return err
		}
		value, err := t.pop1()
		if err != nil {
			return err
		}
		i := index / cffFixedOne
		if i < 0 || i >= cffTransientSize {
			return fmt.Errorf("font: CFF glyph %d stores into transient slot %d", t.gid, i)
		}
		t.transient[i] = value
		return nil
	case cffOpGet:
		index, err := t.pop1()
		if err != nil {
			return err
		}
		i := index / cffFixedOne
		if i < 0 || i >= cffTransientSize {
			return fmt.Errorf("font: CFF glyph %d reads transient slot %d", t.gid, i)
		}
		return t.push(t.transient[i])
	case cffOpIfelse:
		v2, err := t.pop1()
		if err != nil {
			return err
		}
		v1, err := t.pop1()
		if err != nil {
			return err
		}
		s2, err := t.pop1()
		if err != nil {
			return err
		}
		s1, err := t.pop1()
		if err != nil {
			return err
		}
		if v1 <= v2 {
			return t.push(s1)
		}
		return t.push(s2)
	case cffOpRandom:
		// A fixed value keeps the converted font byte identical across runs;
		// the operator only appears in hint programs, which are not applied.
		return t.push(cffFixedOne / 2)
	case cffOpMul:
		b, err := t.pop1()
		if err != nil {
			return err
		}
		a, err := t.pop1()
		if err != nil {
			return err
		}
		return t.push(a * b / cffFixedOne)
	case cffOpSqrt:
		a, err := t.pop1()
		if err != nil {
			return err
		}
		if a < 0 {
			return fmt.Errorf("font: CFF glyph %d takes the square root of a negative number", t.gid)
		}
		return t.push(cffSqrt(a))
	case cffOpDup:
		if len(t.stack) == 0 {
			return errCFFOperands(t.gid, "dup")
		}
		return t.push(t.stack[len(t.stack)-1])
	case cffOpExch:
		b, err := t.pop1()
		if err != nil {
			return err
		}
		a, err := t.pop1()
		if err != nil {
			return err
		}
		if err := t.push(b); err != nil {
			return err
		}
		return t.push(a)
	case cffOpIndex:
		number, err := t.pop1()
		if err != nil {
			return err
		}
		i := int(number / cffFixedOne)
		if i < 0 || i >= len(t.stack) {
			return fmt.Errorf("font: CFF glyph %d indexes operand %d of %d", t.gid, i, len(t.stack))
		}
		return t.push(t.stack[len(t.stack)-1-i])
	case cffOpRoll:
		shift, err := t.pop1()
		if err != nil {
			return err
		}
		number, err := t.pop1()
		if err != nil {
			return err
		}
		t.roll(int(number/cffFixedOne), int(shift/cffFixedOne))
		return nil
	}
	if op < 0x0c00 {
		return fmt.Errorf("font: CFF glyph %d uses the reserved operator %d", t.gid, op)
	}
	return fmt.Errorf("font: CFF glyph %d uses the reserved operator 12 %d", t.gid, op-0x0c00)
}

// roll rotates the top n operands by shift positions, towards the top of the
// stack for a positive shift.
func (t *cffInterp) roll(n, shift int) {
	if n <= 0 || n > len(t.stack) {
		return
	}
	shift = ((shift % n) + n) % n
	if shift == 0 {
		return
	}
	top := t.stack[len(t.stack)-n:]
	rolled := make([]int64, 0, n)
	rolled = append(rolled, top[shift:]...)
	rolled = append(rolled, top[:shift]...)
	copy(top, rolled)
}

// clear drops every operand. The Type 2 stack is shared between a charstring
// and its subroutines, and every stack-clearing operator leaves it empty.
func (t *cffInterp) clear() { t.stack = t.stack[:0] }

func (t *cffInterp) push(v int64) error {
	if len(t.stack) >= cffArgStackSize {
		return fmt.Errorf("font: CFF glyph %d overflows the %d operand positions", t.gid, cffArgStackSize)
	}
	t.stack = append(t.stack, v)
	return nil
}

func (t *cffInterp) pop() int64 {
	v := t.stack[len(t.stack)-1]
	t.stack = t.stack[:len(t.stack)-1]
	return v
}

func (t *cffInterp) pop1() (int64, error) {
	if len(t.stack) < 1 {
		return 0, errCFFOperands(t.gid, "an arithmetic operator")
	}
	return t.pop(), nil
}

// emit records one segment, bounding the points of one glyph.
func (t *cffInterp) emit(op cffOp, args ...cffPoint) error {
	t.points += len(args)
	if t.points > maxGlyphPoints {
		return fmt.Errorf("%w: glyph %d draws more than %d points", ErrLimit, t.gid, maxGlyphPoints)
	}
	segment := cffSegment{Op: op}
	copy(segment.Args[:], args)
	t.segments = append(t.segments, segment)
	return nil
}

// point converts an accumulated position in 16.16 font units into the 26.6
// units of the converter.
func (t *cffInterp) point(x, y int64) cffPoint {
	return cffPoint{X: divRound(x, 1024), Y: divRound(y, 1024)}
}

func cffBool(v bool) int64 {
	if v {
		return cffTrue
	}
	return 0
}

func cffAbs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// cffSqrt returns the square root of a 16.16 fixed-point value.
func cffSqrt(v int64) int64 {
	if v <= 0 {
		return 0
	}
	x := v << 16
	root := x
	guess := (x + 1) / 2
	for guess < root {
		root = guess
		guess = (root + x/root) / 2
	}
	return root
}

// errCFFOperands reports an operator whose operands do not match its
// definition.
func errCFFOperands(gid uint16, op string) error {
	return fmt.Errorf("font: CFF glyph %d: %s has the wrong number of operands", gid, op)
}
