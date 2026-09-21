//go:build !darwin

package mdns

import "log"

// Advertiser is a no-op outside Darwin.
type Advertiser struct{}

func (a *Advertiser) Start(port int, joinURL string) error {
	log.Printf("mDNS: skipped on non-Darwin (would advertise _computepool._tcp port %d join=%s)", port, joinURL)
	return nil
}

func (a *Advertiser) Stop() {}
