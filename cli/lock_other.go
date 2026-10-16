//go:build !windows && !darwin

package main

func hidSetup() {}

// hostLockSymbol is unknown here; the real state is simply not resent.
func hostLockSymbol() byte {
	return 0
}
