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
	// CompletedSteps records which step indices that genuinely completed (done,
	// NOT warn — a warn step is re-attempted on resume).
	CompletedSteps []int `json:"completed_steps"`
	// Version is the installer version string at the time of the run.
	Version string `json:"version"`
	// StepCount is the number of steps in the run that wrote this checkpoint.
	// Resume is only valid when this matches the current run's step count — it
	// guards against step-index drift between versions (indices are positional).
	StepCount int `json:"step_count"`
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

// clearInstallState removes the checkpoint file. Called once an install
// finishes successfully so a COMPLETED install is never mistaken for an
// interrupted one on the next run (which would force fresh/repair instead of
// letting auto-detect choose upgrade). Errors are ignored — a stale file only
// affects mode auto-detection, which the binary+config probe still backstops.
func clearInstallState() {
	os.Remove(stateFilePath())
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

// ── Install-mode detection ────────────────────────────────────────────────────

// existingInstallInfo holds signals used to decide upgrade vs fresh vs repair.
type existingInstallInfo struct {
	// Found is true when an existing Qorven install is detected.
	Found bool
	// BinaryVersion is the version string reported by the existing binary (may be empty).
	BinaryVersion string
}

// detectExistingInstall probes for a COMPLETE prior install. "Found" requires
// the binary AND the config.toml to both exist — a lone binary (e.g. left by a
// fresh install that failed before DB/user setup) does NOT count as a complete
// install, so it won't be misclassified as an upgrade. It does NOT run qorven
// itself so it is safe when the binary is mid-update.
func detectExistingInstall() existingInstallInfo {
	info := existingInstallInfo{}

	binPath := platformBinPath()
	if _, err := os.Stat(binPath); err != nil {
		return info // no binary → definitely fresh
	}

	// Binary exists — try to get its version (best-effort; ignore errors).
	out, err := exec.Command(binPath, "version").CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "v") || strings.Contains(line, "version") {
				info.BinaryVersion = line
				break
			}
		}
	}

	// A complete install also has a config.toml. Without it, this is a partial/
	// failed fresh attempt (binary placed, setup never finished) — NOT an upgrade.
	if _, cErr := os.Stat(filepath.Join(platformConfigDir(), "config.toml")); cErr == nil {
		info.Found = true
	}
	return info
}

// resolveInstallMode returns the effective install mode given the user-supplied
// Config.Mode, the QORVEN_INSTALL_MODE env var, and signals from detectExistingInstall.
//
//   - If Config.Mode is set explicitly (e.g. from a flag), use it.
//   - If QORVEN_INSTALL_MODE env var is set, use it.
//   - Otherwise auto-detect: only choose upgrade when a COMPLETE install exists
//     (binary + config) AND no partial-install checkpoint is in progress. A
//     binary left by a failed fresh attempt → fresh (so PG/user/DB setup runs).
func resolveInstallMode(cfg Config) InstallMode {
	if cfg.Mode != "" {
		return cfg.Mode
	}
	if env := strings.TrimSpace(os.Getenv("QORVEN_INSTALL_MODE")); env != "" {
		switch InstallMode(env) {
		case InstallModeUpgrade, InstallModeRepair, InstallModeFresh:
			return InstallMode(env)
		}
	}
	// If a checkpoint shows a fresh/repair install was interrupted (steps done
	// but not finished), resume that as fresh — never silently upgrade over it.
	if st := loadInstallState(); st != nil && len(st.CompletedSteps) > 0 && st.Mode != string(InstallModeUpgrade) {
		return InstallMode(st.Mode)
	}
	if detectExistingInstall().Found {
		return InstallModeUpgrade
	}
	return InstallModeFresh
}
