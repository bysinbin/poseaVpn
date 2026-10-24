//go:build !windows && !darwin

package guiapp

import "fmt"

func queryServiceState() string { return "notinstalled" }

func (a *App) ServiceInstall() error   { return fmt.Errorf("unsupported platform") }
func (a *App) ServiceStart() error     { return fmt.Errorf("unsupported platform") }
func (a *App) ServiceRestart() error   { return fmt.Errorf("unsupported platform") }
func (a *App) ServiceUninstall() error { return fmt.Errorf("unsupported platform") }
