//go:build darwin

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"poseavpn/pkg/ipc"
)

const (
	launchDaemonPlist = "/Library/LaunchDaemons/com.poseavpn.service.plist"
	launchDaemonLabel = "com.poseavpn.service"
)

func installService() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	absPath, err := filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	_ = os.MkdirAll("/var/log", 0o755)

	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/poseavpn.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/poseavpn.log</string>
</dict>
</plist>
`, launchDaemonLabel, absPath)

	if err := os.WriteFile(launchDaemonPlist, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write %s (run as root / with sudo): %w", launchDaemonPlist, err)
	}

	cmd := exec.Command("launchctl", "load", "-w", launchDaemonPlist)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load failed: %w (%s)", err, string(out))
	}

	fmt.Printf("Service '%s' installed and started successfully via launchd.\n", serviceName)
	return nil
}

func uninstallService() error {
	cmd := exec.Command("launchctl", "unload", "-w", launchDaemonPlist)
	_ = cmd.Run()

	if err := os.Remove(launchDaemonPlist); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove %s: %w", launchDaemonPlist, err)
	}

	fmt.Printf("Service '%s' uninstalled successfully.\n", serviceName)
	return nil
}

func runService() error {
	log.Printf("[PoseaVPN Service] Starting background daemon on macOS (pid %d)...", os.Getpid())

	server, err := ipc.NewServer()
	if err != nil {
		return fmt.Errorf("failed to start IPC server: %w", err)
	}
	defer server.Stop()

	go server.Start()
	log.Printf("[PoseaVPN Service] Daemon running successfully.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	sig := <-sigCh

	log.Printf("[PoseaVPN Service] Signal %v received, shutting down gracefully...", sig)
	return nil
}
