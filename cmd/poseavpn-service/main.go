package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"poseavpn/pkg/ipc"
)

const (
	serviceName        = "PoseaVPN"
	serviceDisplayName = "PoseaVPN Tunnel Service"
	serviceDescription = "Provides secure Active Directory domain-compatible WireGuard and OpenVPN tunneling for non-admin domain users."
)

func runInteractive() {
	fmt.Printf("[PoseaVPN Service] Running in interactive debug mode. Press Ctrl+C to stop.\n")
	server, err := ipc.NewServer()
	if err != nil {
		log.Fatalf("Failed to start IPC server: %v", err)
	}
	defer server.Stop()

	go server.Start()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Printf("\n[PoseaVPN Service] Stopping IPC server...\n")
}

func main() {
	installFlag := flag.Bool("install", false, "Install Service (Run as Administrator / root)")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall Service (Run as Administrator / root)")
	debugFlag := flag.Bool("debug", false, "Run in interactive console debug mode")
	flag.Parse()

	if *installFlag {
		if err := installService(); err != nil {
			log.Fatalf("Error installing service: %v", err)
		}
		return
	}

	if *uninstallFlag {
		if err := uninstallService(); err != nil {
			log.Fatalf("Error uninstalling service: %v", err)
		}
		return
	}

	if *debugFlag {
		runInteractive()
		return
	}

	if err := runService(); err != nil {
		log.Fatalf("Service execution failed: %v", err)
	}
}
