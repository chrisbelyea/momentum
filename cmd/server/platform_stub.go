//go:build !windows

package main

func runAsPlatform() error {
	runServer(nil)
	return nil
}
