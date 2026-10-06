package main

import "testing"

func TestConfiguredListenAddressDefaultsToLoopback(t *testing.T) {
	t.Setenv("LISTEN_ADDR", "")
	got, err := configuredListenAddress("8443")
	if err != nil {
		t.Fatalf("configuredListenAddress returned error: %v", err)
	}
	if got != "127.0.0.1:8443" {
		t.Fatalf("default listen address = %q, want 127.0.0.1:8443", got)
	}
}

func TestConfiguredListenAddressHonorsExplicitAddress(t *testing.T) {
	t.Setenv("LISTEN_ADDR", "0.0.0.0")
	got, err := configuredListenAddress("9443")
	if err != nil {
		t.Fatalf("configuredListenAddress returned error: %v", err)
	}
	if got != "0.0.0.0:9443" {
		t.Fatalf("explicit listen address = %q, want 0.0.0.0:9443", got)
	}
}

func TestConfiguredListenAddressRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv("LISTEN_ADDR", "bad/path")
	if _, err := configuredListenAddress("8443"); err == nil {
		t.Fatal("expected invalid LISTEN_ADDR to be rejected")
	}
	t.Setenv("LISTEN_ADDR", "127.0.0.1")
	if _, err := configuredListenAddress("99999"); err == nil {
		t.Fatal("expected invalid PORT to be rejected")
	}
}
