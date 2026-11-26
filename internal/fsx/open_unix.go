//go:build unix

package fsx

import (
	"os"
	"syscall"
)

// openInRoot opens name inside root without waiting for a writer: O_NONBLOCK
// makes a FIFO open at once even when no writer is connected, and the caller
// rejects it through the descriptor before reading. For a regular file the flag
// has no effect on Unix reads.
func openInRoot(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

// openPath opens an operating-system path with the same non-blocking flag.
func openPath(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
