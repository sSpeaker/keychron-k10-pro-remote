//go:build !windows && !darwin

package main

// hostLockSymbol is unknown here; the real state is simply not resent.
func hostLockSymbol() byte {
	return 0
}
