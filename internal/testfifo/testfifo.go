// Package testfifo creates POSIX FIFOs for tests, in one place.
//
// Go cannot import test files, so the helper lives in a normal package. Only
// tests import it: the production conversion path never reads this package.
// A platform whose syscall package cannot create a POSIX FIFO skips the test
// that asks for one.
package testfifo
