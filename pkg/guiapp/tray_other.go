//go:build !windows && !darwin

package guiapp

func trayRunning() bool { return false }
func ensureTray()       {}
