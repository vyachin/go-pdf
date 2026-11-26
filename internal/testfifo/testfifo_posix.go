//go:build linux || darwin || dragonfly || freebsd || netbsd || openbsd

package testfifo

import (
	"syscall"
	"testing"
)

// Create makes a FIFO at path. These are the platforms whose syscall package
// exposes Mkfifo; the test skips when the filesystem refuses to create one, so
// a build without FIFO support reports the gap instead of failing.
func Create(t testing.TB, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("cannot create a FIFO: %v", err)
	}
}
