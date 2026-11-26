//go:build !unix

package fsx

import "os"

// openInRoot opens name inside root. The platform has neither POSIX FIFOs nor
// O_NONBLOCK for files, so a plain open cannot wait for a writer; the
// regular-file gate still classifies whatever was opened.
func openInRoot(root *os.Root, name string) (*os.File, error) {
	return root.Open(name)
}

// openPath opens an operating-system path on a platform without O_NONBLOCK.
func openPath(path string) (*os.File, error) {
	return os.Open(path)
}
