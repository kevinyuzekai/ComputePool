package netutil

import (
	"net"
	"strings"
)

// PrimaryLANIPv4 returns a best-effort non-loopback IPv4 for join URLs.
func PrimaryLANIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	var fallback string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		// Prefer common LAN names.
		prefer := strings.HasPrefix(name, "en") || strings.HasPrefix(name, "eth") ||
			strings.HasPrefix(name, "wlan") || strings.HasPrefix(name, "wl")
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ip := ipFromAddr(a)
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			s := ip.String()
			if prefer {
				return s
			}
			if fallback == "" {
				fallback = s
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	return "127.0.0.1"
}

func ipFromAddr(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		return nil
	}
}
