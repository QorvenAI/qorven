// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	osexec "os/exec"
	"regexp"
	"strings"
)

// networkStatus holds the payload returned by GET /v1/network/status.
type networkStatus struct {
	TailscaleInstalled bool   `json:"tailscale_installed"`
	TailscaleIP        string `json:"tailscale_ip"`
	TailscaleHostname  string `json:"tailscale_hostname"`
	BindMode           string `json:"bind_mode"`
	WebListen          string `json:"web_listen"`
	APIListen          string `json:"api_listen"`
}

// tailscaleIP runs `tailscale ip -4` and returns the first IPv4 address, or "".
func tailscaleIP() string {
	out, err := osexec.Command("tailscale", "ip", "-4").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// tailscaleHostname runs `tailscale status --json` and returns the Self.HostName, or "".
func tailscaleHostname() string {
	out, err := osexec.Command("tailscale", "status", "--json").Output()
	if err != nil {
		return ""
	}
	var status struct {
		Self struct {
			HostName string `json:"HostName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return ""
	}
	return status.Self.HostName
}

// bindModeFromAddr derives bind_mode from a web listen address.
func bindModeFromAddr(addr string) string {
	switch {
	case strings.HasPrefix(addr, "100."):
		return "tailscale"
	case strings.HasPrefix(addr, "127.") || strings.HasPrefix(addr, "localhost"):
		return "localhost"
	default:
		return "public"
	}
}

// currentNetworkStatus builds a networkStatus snapshot from live system state.
func (gw *Gateway) currentNetworkStatus() networkStatus {
	_, err := osexec.LookPath("tailscale")
	installed := err == nil

	var ip, hostname string
	if installed {
		ip = tailscaleIP()
		if ip != "" {
			hostname = tailscaleHostname()
		}
	}

	webListen := ""
	apiListen := ""
	if gw.cfg != nil {
		webListen = gw.cfg.Server.WebListen
		apiListen = gw.cfg.Server.APIListen
		if apiListen == "" {
			apiListen = gw.cfg.Server.Listen
		}
		if apiListen == "" {
			apiListen = "127.0.0.1:4200"
		}
	}

	return networkStatus{
		TailscaleInstalled: installed,
		TailscaleIP:        ip,
		TailscaleHostname:  hostname,
		BindMode:           bindModeFromAddr(webListen),
		WebListen:          webListen,
		APIListen:          apiListen,
	}
}

// persistWebListen rewrites the web_listen line in config.toml.
// If the file cannot be updated it logs a warning but does not return an error
// so the HTTP response still succeeds.
func (gw *Gateway) persistWebListen(newAddr string) {
	if gw.cfg == nil || gw.cfg.ConfigPath == "" {
		slog.Warn("network.persist: no config path — add web_listen manually", "web_listen", newAddr)
		return
	}

	data, err := os.ReadFile(gw.cfg.ConfigPath)
	if err != nil {
		slog.Warn("network.persist: cannot read config", "path", gw.cfg.ConfigPath, "err", err)
		return
	}

	re := regexp.MustCompile(`(?m)^web_listen\s*=\s*"[^"]*"`)
	newLine := fmt.Sprintf(`web_listen = "%s"`, newAddr)
	updated := re.ReplaceAllString(string(data), newLine)

	if updated == string(data) {
		// Line not found — scan for [server] section and append, or just warn.
		slog.Warn("network.persist: web_listen not found in config — add it manually",
			"path", gw.cfg.ConfigPath, "suggested", newLine)
		return
	}

	if err := os.WriteFile(gw.cfg.ConfigPath, []byte(updated), 0600); err != nil {
		slog.Warn("network.persist: cannot write config", "path", gw.cfg.ConfigPath, "err", err)
		return
	}
	slog.Info("network.persist: web_listen updated", "path", gw.cfg.ConfigPath, "addr", newAddr)
}

// handleNetworkStatus handles GET /v1/network/status.
func (gw *Gateway) handleNetworkStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, gw.currentNetworkStatus())
}

// handleNetworkTailscale handles POST /v1/network/tailscale.
func (gw *Gateway) handleNetworkTailscale(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action  string `json:"action"`
		AuthKey string `json:"auth_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	switch req.Action {
	case "install":
		// Run the official Tailscale one-liner install script.
		cmd := osexec.CommandContext(r.Context(), "sh", "-c", "curl -fsSL https://tailscale.com/install.sh | sh")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("tailscale install failed: %s", err.Error()),
			})
			return
		}
		slog.Info("network.tailscale: installed")

		if req.AuthKey != "" {
			up := osexec.CommandContext(r.Context(), "tailscale", "up", "--auth-key="+req.AuthKey)
			up.Stdout = os.Stdout
			up.Stderr = os.Stderr
			if err := up.Run(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": fmt.Sprintf("tailscale up failed: %s", err.Error()),
				})
				return
			}
			slog.Info("network.tailscale: authenticated")
		}

		writeJSON(w, http.StatusOK, gw.currentNetworkStatus())

	case "bind":
		ip := tailscaleIP()
		if ip == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "tailscale not connected — cannot determine IP",
			})
			return
		}

		// Preserve existing port; default to 443.
		port := "443"
		if gw.cfg != nil && gw.cfg.Server.WebListen != "" {
			if idx := strings.LastIndex(gw.cfg.Server.WebListen, ":"); idx >= 0 {
				port = gw.cfg.Server.WebListen[idx+1:]
			}
		}
		newAddr := ip + ":" + port
		if gw.cfg != nil {
			gw.cfg.Server.WebListen = newAddr
		}
		gw.persistWebListen(newAddr)
		slog.Info("network.tailscale: bound", "addr", newAddr)

		st := gw.currentNetworkStatus()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    st,
			"message":   "restart required to apply bind changes",
			"web_listen": newAddr,
		})

	case "unbind":
		// Preserve existing port; default to 443.
		port := "443"
		if gw.cfg != nil && gw.cfg.Server.WebListen != "" {
			if idx := strings.LastIndex(gw.cfg.Server.WebListen, ":"); idx >= 0 {
				port = gw.cfg.Server.WebListen[idx+1:]
			}
		}
		newAddr := "0.0.0.0:" + port
		if gw.cfg != nil {
			gw.cfg.Server.WebListen = newAddr
		}
		gw.persistWebListen(newAddr)
		slog.Info("network.tailscale: unbound", "addr", newAddr)

		st := gw.currentNetworkStatus()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    st,
			"message":   "restart required to apply bind changes",
			"web_listen": newAddr,
		})

	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("unknown action %q — expected install, bind, or unbind", req.Action),
		})
	}
}

