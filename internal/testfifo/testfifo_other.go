//go:build !(linux || darwin || dragonfly || freebsd || netbsd || openbsd)

package testfifo

import "testing"

// Create skips the test: the platform does not create POSIX FIFOs through the
// syscall package, or does not create them at all, so the FIFO case cannot be
// exercised here.
func Create(t testing.TB, path string) {
	t.Helper()
	t.Skip("the platform does not create POSIX FIFOs")
}
