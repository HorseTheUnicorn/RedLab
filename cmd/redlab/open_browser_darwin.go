package main

import "os/exec"

func openBrowserURL(address string) error {
	// #nosec G204 -- the executable is fixed and address is a loopback URL.
	return exec.Command("open", address).Start()
}
