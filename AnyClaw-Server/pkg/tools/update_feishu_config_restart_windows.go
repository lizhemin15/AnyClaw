//go:build windows

package tools

// scheduleRestart is a no-op on Windows; user must restart manually.
func scheduleRestart() {}
