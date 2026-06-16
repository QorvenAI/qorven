// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package installer

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ── Install-state checkpoint ──────────────────────────────────────────────────

// installState is written to <platformConfigDir>/.install-state.json after
// each step completes. It lets a re-run resume from the first incomplete step
// instead of re-running the full sequence.
type installState struct {
	// CompletedSteps records which step indices completed (done or warn).
	CompletedSteps []int `json:"completed_steps"`
	// Version is the installer version string at the time of the run.
	Version string `json:"version"`
	// Mode is the install mode ("fresh", "upgrade", "repair").
	Mode string `json:"mode"`
	// Config captures the flags chosen by the user so a resumed run uses the
	// same settings.
	Config struct {
		Port          int    `json:"port"`
		DataDir       string `json:"data_dir"`
		SkipPG        bool   `json:"skip_pg"`
		SkipDocker    bool   `json:"skip_docker"`
		SkipTailscale bool   `json:"skip_tailscale"`
		SkipNginx     bool   `json:"skip_nginx"`
	} `json:"config"`
	// UpdatedAt is the timestamp of the last checkpoint write.
	UpdatedAt time.Time `json:"updated_at"`
}

// stateFilePath returns the path to .install-state.json.
func stateFilePath() string {
	return filepath.Join(platformConfigDir(), ".install-state.json")
}

// loadInstallState reads the state file and returns it. Returns nil (not an
// error) when the file is absent or corrupt — callers treat that as "no prior
// state" and run the full sequence.
func loadInstallState() *installState {
	data, err := os.ReadFile(stateFilePath())
	if err != nil {
		return nil
	}
	var s installState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil // corrupt — ignore safely
	}
	return &s
}

// saveInstallState persists the state file. Errors are silently ignored: the
// state file is an optimisation (faster re-runs), not a correctness requirement.
func saveInstallState(s *installState) {
	s.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(stateFilePath()), 0755)
	os.WriteFile(stateFilePath(), data, 0644) //nolint:errcheck
}

// stepCompletedInState returns true if the given step index appears in the
// state's CompletedSteps list.
func stepCompletedInState(s *installState, idx int) bool {
	if s == nil {
		return false
	}
	for _, i := range s.CompletedSteps {
		if i == idx {
			return true
		}
	}
	return false
}

// markStepComplete records idx as completed in the state and persists it.
func markStepComplete(s *installState, idx int) {
	if s == nil {
		return
	}
	if !stepCompletedInState(s, idx) {
		s.CompletedSteps = append(s.CompletedSteps, idx)
	}
	saveInstallState(s)
}

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
