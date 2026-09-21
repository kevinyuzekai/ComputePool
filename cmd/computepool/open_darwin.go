//go:build darwin

package main

import (
	"fmt"
	"os/exec"
)

func openBrowser(url string) error {
	return exec.Command("open", url).Start()
}

func notifyUIReady(url string) {
	msg := fmt.Sprintf("ComputePool 已启动，请打开 %s", url)
	script := fmt.Sprintf(`display notification %q with title "ComputePool"`, msg)
	_ = exec.Command("osascript", "-e", script).Start()
}
