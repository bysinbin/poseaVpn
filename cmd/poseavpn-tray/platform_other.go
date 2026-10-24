//go:build !windows && !darwin

package main

import "time"

const autostartMenuTitle = "Başlangıçta çalıştır"

func singleInstanceLock() (func(), bool, error) { return func() {}, false, nil }
func waitForTaskbar(timeout time.Duration) bool   { return true }
func closeGUIs()                                 {}
func launchMulti()                               { launch("poseavpn-multi") }
func launchSingle()                              { launch("poseavpn-single") }
func launch(exeName string)                      {}
func messageBox(title, text string, flags uint32) {}
func showError(title, text string)               {}
func enableAutostartOnFirstRun()                 {}
func autostartEnabled() bool                     { return false }
func toggleAutostart()                           {}
