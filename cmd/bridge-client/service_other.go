//go:build !windows

package main

func runWindowsServiceIfNeeded(_ string) bool {
	return false
}
