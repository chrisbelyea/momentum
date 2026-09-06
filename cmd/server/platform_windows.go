//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/svc"
)

func runAsPlatform() error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("detect Windows service context: %w", err)
	}
	if !isService {
		runServer(nil)
		return nil
	}
	serviceName := getEnv("MOMENTUM_SERVICE_NAME", "Momentum")
	return svc.Run(serviceName, momentumService{})
}

type momentumService struct{}

var serviceServer = runServer

func (momentumService) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	stop := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		serviceServer(stop)
		close(done)
	}()

	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for request := range requests {
		switch request.Cmd {
		case svc.Interrogate:
			changes <- request.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			stop <- os.Interrupt
			<-done
			return false, 0
		}
	}

	return false, 0
}
