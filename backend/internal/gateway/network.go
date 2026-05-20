// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	osexec "os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/qorvenai/qorven/internal/config"
)

// networkStatus holds the payload returned by GET /v1/network/status.
type networkStatus struct {
	TailscaleInstalled bool   `json:"tailscale_installed"`
	TailscaleIP        string `json:"tailscale_ip"`
	TailscaleHostname  string `json:"tailscale_hostname"`
	BindMode           string `json:"bind_mode"`
	Listen             string `json:"listen"`
	// Keep old fields for backward-compat with old frontends.
	WebListen string `json:"web_listen"`
	APIListen string `json:"api_listen"`
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

// bindModeFromAddr derives bind_mode from a listen address.
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

	listen := ""
	if gw.cfg != nil {
		listen = gw.cfg.Server.Listen
		if listen == "" {
			listen = fmt.Sprintf("0.0.0.0:%d", config.DefaultPort)
		}
	}

	return networkStatus{
		TailscaleInstalled: installed,
		TailscaleIP:        ip,
		TailscaleHostname:  hostname,
		BindMode:           bindModeFromAddr(listen),
		Listen:             listen,
		// Mirror into old fields so old frontends still get valid addresses.
		WebListen: listen,
		APIListen: listen,
	}
}

// persistWebListen rewrites the web_listen line in config.toml.
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

// persistListen rewrites the listen line in config.toml.
func (gw *Gateway) persistListen(newAddr string) {
	if gw.cfg == nil || gw.cfg.ConfigPath == "" {
		slog.Warn("network.persist: no config path — add listen manually", "listen", newAddr)
		return
	}
	data, err := os.ReadFile(gw.cfg.ConfigPath)
	if err != nil {
		slog.Warn("network.persist: cannot read config", "path", gw.cfg.ConfigPath, "err", err)
		return
	}
	re := regexp.MustCompile(`(?m)^listen\s*=\s*"[^"]*"`)
	newLine := fmt.Sprintf(`listen = "%s"`, newAddr)
	updated := re.ReplaceAllString(string(data), newLine)
	if updated == string(data) {
		slog.Warn("network.persist: listen not found in config — add it manually",
			"path", gw.cfg.ConfigPath, "suggested", newLine)
		return
	}
	if err := os.WriteFile(gw.cfg.ConfigPath, []byte(updated), 0600); err != nil {
		slog.Warn("network.persist: cannot write config", "path", gw.cfg.ConfigPath, "err", err)
		return
	}
	slog.Info("network.persist: listen updated", "path", gw.cfg.ConfigPath, "addr", newAddr)
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

		// Preserve existing port from Listen (canonical), fall back to WebListen; default to 443.
		port := "443"
		if gw.cfg != nil {
			listenAddr := gw.cfg.Server.Listen
			if listenAddr == "" {
				listenAddr = gw.cfg.Server.WebListen
			}
			if listenAddr != "" {
				if idx := strings.LastIndex(listenAddr, ":"); idx >= 0 {
					port = listenAddr[idx+1:]
				}
			}
		}
		newAddr := ip + ":" + port
		if gw.cfg != nil {
			gw.cfg.Server.Listen = newAddr
			gw.cfg.Server.WebListen = newAddr
		}
		gw.persistWebListen(newAddr)
		gw.persistListen(newAddr)
		slog.Info("network.tailscale: bound", "addr", newAddr)

		st := gw.currentNetworkStatus()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     st,
			"message":    "restart required to apply bind changes",
			"web_listen": newAddr,
		})

	case "unbind":
		// Preserve existing port from Listen (canonical), fall back to WebListen; default to 443.
		port := "443"
		if gw.cfg != nil {
			listenAddr := gw.cfg.Server.Listen
			if listenAddr == "" {
				listenAddr = gw.cfg.Server.WebListen
			}
			if listenAddr != "" {
				if idx := strings.LastIndex(listenAddr, ":"); idx >= 0 {
					port = listenAddr[idx+1:]
				}
			}
		}
		newAddr := "0.0.0.0:" + port
		if gw.cfg != nil {
			gw.cfg.Server.Listen = newAddr
			gw.cfg.Server.WebListen = newAddr
		}
		gw.persistWebListen(newAddr)
		gw.persistListen(newAddr)
		slog.Info("network.tailscale: unbound", "addr", newAddr)

		st := gw.currentNetworkStatus()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     st,
			"message":    "restart required to apply bind changes",
			"web_listen": newAddr,
		})

	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("unknown action %q — expected install, bind, or unbind", req.Action),
		})
	}
}

// handleCheckPort handles GET /v1/admin/system/check-port?port=N
// Returns whether the port is available for binding on this machine.
func (gw *Gateway) handleCheckPort(w http.ResponseWriter, r *http.Request) {
	portStr := r.URL.Query().Get("port")
	if portStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "port parameter required"})
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid port number"})
		return
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"port":      port,
			"available": false,
			"reason":    err.Error(),
		})
		return
	}
	ln.Close()
	writeJSON(w, http.StatusOK, map[string]any{
		"port":      port,
		"available": true,
	})
}
