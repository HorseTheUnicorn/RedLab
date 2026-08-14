package main

import "os/exec"

func openBrowserURL(address string) error {
	// #nosec G204 -- the executable and handler are fixed; address is the
	// loopback URL constructed by the local serve command.
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", address).Start()
}
