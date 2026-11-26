//go:build !race

package engine

// raceEnabled reports that the race detector is active; see race_test.go.
const raceEnabled = false
