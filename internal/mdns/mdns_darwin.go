//go:build darwin

package mdns

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
)

// Advertiser registers _computepool._tcp via dns-sd when available.
type Advertiser struct {
	cmd *exec.Cmd
}

// Start advertises the hub. No-op if dns-sd missing.
func (a *Advertiser) Start(port int, joinURL string) error {
	if _, err := exec.LookPath("dns-sd"); err != nil {
		log.Printf("mDNS: dns-sd not found, skip Bonjour (_computepool._tcp)")
		return nil
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "ComputePool"
	}
	name := fmt.Sprintf("ComputePool (%s)", host)
	// dns-sd -R name type domain port [key=value ...]
	a.cmd = exec.Command("dns-sd", "-R", name, "_computepool._tcp", "local",
		strconv.Itoa(port), "join="+joinURL, "path=/")
	a.cmd.Stdout = nil
	a.cmd.Stderr = nil
	if err := a.cmd.Start(); err != nil {
		log.Printf("mDNS: start failed: %v", err)
		return err
	}
	log.Printf("mDNS: advertising %s._computepool._tcp.local. port %d", name, port)
	go func() {
		_ = a.cmd.Wait()
	}()
	return nil
}

// Stop terminates dns-sd.
func (a *Advertiser) Stop() {
	if a.cmd != nil && a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
		a.cmd = nil
	}
}
