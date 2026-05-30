// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package installer

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"os/exec"
	"strings"
)

// ── Shell helpers ─────────────────────────────────────────────────────────────

func runQuiet(name string, args ...string) error {
	c := exec.Command(name, args...)
	return c.Run()
}

func runSilent(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func randHex(bytes int) string {
	b := make([]byte, bytes)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ── IP detection ──────────────────────────────────────────────────────────────

type ipResult struct {
	publicURL string
	wanIP     string
	lanIPs    []string
	behindNAT bool
}

func detectIPs() ipResult {
	var publicIface, privateIface []string
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			if isPrivateIP(ip) {
				privateIface = append(privateIface, ip.String())
			} else {
				publicIface = append(publicIface, ip.String())
			}
		}
	}
	if len(publicIface) > 0 {
		return ipResult{publicURL: publicIface[0], lanIPs: append(publicIface[1:], privateIface...)}
	}
	r := ipResult{behindNAT: true, lanIPs: privateIface}
	if ip, err := runSilent("curl", "-sf", "--connect-timeout", "1",
		"http://169.254.169.254/latest/meta-data/public-ipv4"); err == nil && isValidIP(ip) {
		r.publicURL = strings.TrimSpace(ip)
		r.wanIP = r.publicURL
		r.behindNAT = false
		r.lanIPs = privateIface
		return r
	}
	if ip, err := runSilent("curl", "-sf", "--connect-timeout", "3",
		"https://api.ipify.org"); err == nil && isValidIP(ip) {
		r.wanIP = strings.TrimSpace(ip)
	}
	if len(privateIface) > 0 {
		r.publicURL = privateIface[0]
		r.lanIPs = privateIface[1:]
	}
	return r
}

func isPrivateIP(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	if ip[0] == 10 {
		return true
	}
	if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		return true
	}
	if ip[0] == 192 && ip[1] == 168 {
		return true
	}
	if ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127 {
		return true
	}
	return false
}

func isValidIP(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && net.ParseIP(s) != nil
}

func isPrivateHostIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return isPrivateIP(ip)
}

func detectMode(urlInput string, ips ipResult, tsIP string) string {
	if tsIP != "" {
		return "tailscale"
	}
	host := urlInput
	if strings.Contains(host, "://") {
		host = strings.SplitN(host, "://", 2)[1]
	}
	host = strings.SplitN(host, "/", 2)[0]
	if strings.HasPrefix(host, "100.") {
		return "tailscale"
	}
	if ips.behindNAT {
		return "nat"
	}
	if host == "localhost" || strings.HasPrefix(host, "127.") ||
		strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "100.64.") || isPrivateHostIP(host) {
		return "local"
	}
	return "public"
}
