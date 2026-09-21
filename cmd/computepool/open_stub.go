//go:build !darwin

package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).Start()
	default:
		fmt.Println("Open:", url)
		return nil
	}
}

func notifyUIReady(url string) {
	_ = url
}
