//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"poseavpn/pkg/ipc"
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

func runService() error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("failed to detect service mode: %w", err)
	}

	if isService {
		return svc.Run(serviceName, &poseaService{})
	}

	fmt.Printf("PoseaVPN Service Manager\n")
	fmt.Printf("Usage:\n")
	fmt.Printf("  poseavpn-service.exe --install    (Install and start as Windows Service)\n")
	fmt.Printf("  poseavpn-service.exe --uninstall  (Stop and remove Windows Service)\n")
	fmt.Printf("  poseavpn-service.exe --debug      (Run in console debug mode)\n")
	return nil
}
