package engine

import "testing"

// TestGateProbe fails on purpose. It exists only on a scratch branch to prove
// that a failing unit test turns the gate red and blocks the pull request.
func TestGateProbe(t *testing.T) {
	t.Fatal("deliberate failure to verify that the gate catches a failing unit test")
}
