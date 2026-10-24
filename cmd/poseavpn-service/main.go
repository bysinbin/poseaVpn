package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"poseavpn/pkg/ipc"
)

const (
	serviceName        = "PoseaVPN"
	serviceDisplayName = "PoseaVPN Tunnel Service"
	serviceDescription = "Provides secure Active Directory domain-compatible WireGuard tunneling for non-admin domain users."
)

type poseaService struct {
	ipcServer *ipc.Server
}

func (s *poseaService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	server, err := ipc.NewServer()
	if err != nil {
		log.Printf("[Service] Failed to start IPC server: %v", err)
		return false, 1
	}
	s.ipcServer = server

	go s.ipcServer.Start()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	log.Printf("[Service] %s running successfully.", serviceName)

	for req := range r {
		switch req.Cmd {
		case svc.Interrogate:
			changes <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			log.Printf("[Service] Stop signal received, shutting down...")
			changes <- svc.Status{State: svc.StopPending}
			s.ipcServer.Stop()
			changes <- svc.Status{State: svc.Stopped}
			return false, 0
		default:
			log.Printf("[Service] Unexpected service control request #%d", req.Cmd)
		}
	}

	return false, 0
}

func installService() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	absPath, err := filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to Service Control Manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s is already installed", serviceName)
	}

	s, err = m.CreateService(serviceName, absPath, mgr.Config{
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
		StartType:   mgr.StartAutomatic,
	})
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer s.Close()

	// Start the service immediately
	_ = s.Start()

	fmt.Printf("Service '%s' installed and started successfully.\n", serviceName)
	return nil
}


func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to Service Control Manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", serviceName)
	}
	defer s.Close()

	// Stop service if running
	status, err := s.Control(svc.Stop)
	if err == nil {
		timeout := time.Now().Add(10 * time.Second)
		for status.State != svc.Stopped {
			if time.Now().After(timeout) {
				break
			}
			time.Sleep(300 * time.Millisecond)
			status, err = s.Query()
			if err != nil {
				break
			}
		}
	}

	if err := s.Delete(); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	fmt.Printf("Service '%s' uninstalled successfully.\n", serviceName)
	return nil
}

func runInteractive() {
	fmt.Printf("[PoseaVPN Service] Running in interactive debug mode. Press Ctrl+C to stop.\n")
	server, err := ipc.NewServer()
	if err != nil {
		log.Fatalf("Failed to start IPC server: %v", err)
	}
	defer server.Stop()

	server.Start()
}

func main() {
	installFlag := flag.Bool("install", false, "Install Windows Service (Run as Administrator)")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall Windows Service (Run as Administrator)")
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

	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("Failed to detect service mode: %v", err)
	}

	if isService {
		if err := svc.Run(serviceName, &poseaService{}); err != nil {
			log.Fatalf("Service execution failed: %v", err)
		}
	} else {
		// Output help if run from console without arguments
		fmt.Printf("PoseaVPN Service Manager\n")
		fmt.Printf("Usage:\n")
		fmt.Printf("  poseavpn-service.exe --install    (Install and start as Windows Service)\n")
		fmt.Printf("  poseavpn-service.exe --uninstall  (Stop and remove Windows Service)\n")
		fmt.Printf("  poseavpn-service.exe --debug      (Run in console debug mode)\n")
	}
}
