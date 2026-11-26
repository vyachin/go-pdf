package budget

// bytesPerUnit is the number of input bytes one work unit may materialise.
// Every stage whose memory grows with its input charges
// ceil(n / bytesPerUnit) + 1 units before it allocates, so the budget bounds
// memory as well as CPU. The extra unit keeps a zero-byte stage from being
// free and makes the smallest charge positive. The invariant test in
// internal/engine asserts the bound and names the stage that crosses it.
const bytesPerUnit = 4096

// ChargeBytes records the work of materialising n bytes: one unit per started
// block of bytesPerUnit bytes, plus one for the allocation itself. Call it
// before the allocation it describes. A nil guard accepts everything, like
// Add.
func (g *Guard) ChargeBytes(n int) error {
	if n < 0 {
		n = 0
	}
	return g.Add(uint64((n+bytesPerUnit-1)/bytesPerUnit) + 1)
}
