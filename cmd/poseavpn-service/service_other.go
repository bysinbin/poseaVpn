//go:build !windows && !darwin

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"poseavpn/pkg/ipc"
)

func installService() error   { return fmt.Errorf("unsupported platform") }
func uninstallService() error { return fmt.Errorf("unsupported platform") }

func runService() error {
	server, err := ipc.NewServer()
	if err != nil {
		return err
	}
	defer server.Stop()
	go server.Start()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	return nil
}
