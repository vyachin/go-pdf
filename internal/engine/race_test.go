//go:build race

package engine

// raceEnabled reports that the race detector is active. Its shadow memory is
// charged to the process allocator, so an allocation bound measured under it
// would describe the detector, not the conversion.
const raceEnabled = true
