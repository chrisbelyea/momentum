//go:build windows

package main

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestMomentumServiceStopsTheApplication(t *testing.T) {
	original := serviceServer
	t.Cleanup(func() { serviceServer = original })
	serviceServer = func(stop <-chan os.Signal) { <-stop }

	requests := make(chan svc.ChangeRequest, 1)
	changes := make(chan svc.Status, 3)
	done := make(chan struct{})
	go func() {
		momentumService{}.Execute(nil, requests, changes)
		close(done)
	}()

	if status := <-changes; status.State != svc.StartPending {
		t.Fatalf("first service status = %v, want start pending", status.State)
	}
	if status := <-changes; status.State != svc.Running {
		t.Fatalf("second service status = %v, want running", status.State)
	}
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	if status := <-changes; status.State != svc.StopPending {
		t.Fatalf("stop service status = %v, want stop pending", status.State)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("service did not return after stop request")
	}
}

func TestConfiguredServiceName(t *testing.T) {
	t.Setenv("MOMENTUM_SERVICE_NAME", "Momentum-Test")
	if got := getEnv("MOMENTUM_SERVICE_NAME", "Momentum"); got != "Momentum-Test" {
		t.Fatalf("configured service name = %q, want Momentum-Test", got)
	}
}
