// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DevServer manages a development server process for a project.
type DevServer struct {
	ProjectID string
	Port      int
	Cmd       *exec.Cmd
	Cancel    context.CancelFunc
	StartedAt time.Time
	Framework string
	Path      string // absolute path of the project directory
}

// PreviewManager manages dev server processes for live preview.
// Each project can have one active dev server. The manager handles:
// - Auto-detecting the framework and start command
// - Allocating a free port
// - Starting/stopping the dev server
// - Reverse proxying requests to the dev server
type PreviewManager struct {
	mu      sync.RWMutex
	servers map[string]*DevServer // projectID → server
	basePort int
}

func NewPreviewManager() *PreviewManager {
	return &PreviewManager{
		servers:  make(map[string]*DevServer),
		basePort: 9100,
	}
}

// StartDevServer starts a dev server for the given project path.
// It auto-detects the framework and appropriate start command.
func (pm *PreviewManager) StartDevServer(projectID, projectPath string) (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Already running?
	if srv, ok := pm.servers[projectID]; ok {
		return srv.Port, nil
	}

	framework, cmd := detectFramework(projectPath)
	if cmd == "" {
		return 0, fmt.Errorf("no dev server detected for project")
	}

	port, err := pm.allocatePort()
	if err != nil {
		return 0, fmt.Errorf("no free port: %w", err)
	}

	// Inject port into command
	startCmd := injectPort(cmd, port, framework)

	// Part B: HMR config injection.
	//
	// The browser loads the preview iframe at /v1/preview/{id}/... through the
	// gateway.  Vite's HMR client (injected into the page) tries to open a
	// WebSocket back to its dev server.  Without guidance it will connect to the
	// wrong origin/port and be blocked.  We write a thin vite.config.preview.mjs
	// that wraps the project's own config and overrides server.hmr so the client
	// dials the gateway path instead of the raw vite port.
	//
	// For Next.js the WS tunnel (see proxy_preview.go) handles HMR transparently
	// because Next sets the HMR websocket path to /_next/webpack-hmr, which the
	// tunnel forwards correctly without extra configuration.
	if framework == "vite" {
		if viteConfigErr := writeViteHMRConfig(projectPath, projectID); viteConfigErr != nil {
			slog.Warn("preview.hmr_config.write_failed",
				"project", projectID, "err", viteConfigErr)
			// Non-fatal: proceed with the original command; WS tunnel still works
			// for projects where Vite's client happens to guess the right path.
		} else {
			// Point vite at the generated config instead of the project default.
			startCmd = strings.ReplaceAll(startCmd, "npx vite ", "npx vite --config vite.config.preview.mjs ")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	proc := exec.CommandContext(ctx, "sh", "-c", startCmd)
	proc.Dir = projectPath
	proc.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", port),
		fmt.Sprintf("DEV_PORT=%d", port),
		"NODE_ENV=development",
		"BROWSER=none",
	)
	proc.Stdout = nil // suppress output
	proc.Stderr = nil

	if err := proc.Start(); err != nil {
		cancel()
		return 0, fmt.Errorf("start dev server: %w", err)
	}

	srv := &DevServer{
		ProjectID: projectID,
		Port:      port,
		Cmd:       proc,
		Cancel:    cancel,
		StartedAt: time.Now(),
		Framework: framework,
		Path:      projectPath,
	}
	pm.servers[projectID] = srv

	// Wait for server to be ready (up to 30s)
	go pm.waitForReady(srv)

	slog.Info("preview.dev_server.started",
		"project", projectID, "port", port, "framework", framework, "cmd", startCmd)

	return port, nil
}

// StopDevServer stops the dev server for a project.
func (pm *PreviewManager) StopDevServer(projectID string) {
	pm.mu.Lock()
	srv, ok := pm.servers[projectID]
	if ok {
		delete(pm.servers, projectID)
	}
	pm.mu.Unlock()

	if ok && srv.Cancel != nil {
		srv.Cancel()
		if srv.Cmd.Process != nil {
			srv.Cmd.Process.Kill()
		}
		// Remove the generated Vite HMR config override if one was written.
		if srv.Framework == "vite" && srv.Path != "" {
			removeViteHMRConfig(srv.Path)
		}
		slog.Info("preview.dev_server.stopped", "project", projectID)
	}
}

// GetPort returns the dev server port for a project, or 0 if not running.
func (pm *PreviewManager) GetPort(projectID string) int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if srv, ok := pm.servers[projectID]; ok {
		return srv.Port
	}
	return 0
}

// IsRunning returns true if the project has an active dev server.
func (pm *PreviewManager) IsRunning(projectID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	_, ok := pm.servers[projectID]
	return ok
}

// ProxyHandler returns a WebSocket-aware http.Handler that reverse-proxies to the
// project's dev server. See proxy_preview.go for the full implementation.
func (pm *PreviewManager) ProxyHandler(projectID string) http.Handler {
	return previewProxyHandler(pm, projectID)
}

// ListServers returns info about all running dev servers.
func (pm *PreviewManager) ListServers() []map[string]any {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var result []map[string]any
	for _, srv := range pm.servers {
		result = append(result, map[string]any{
			"project_id": srv.ProjectID,
			"port":       srv.Port,
			"framework":  srv.Framework,
			"uptime_sec": int(time.Since(srv.StartedAt).Seconds()),
		})
	}
	return result
}

func (pm *PreviewManager) allocatePort() (int, error) {
	// Try ports starting from basePort
	for port := pm.basePort; port < pm.basePort+100; port++ {
		// Check if port is already in use by our servers
		inUse := false
		for _, srv := range pm.servers {
			if srv.Port == port {
				inUse = true
				break
			}
		}
		if inUse {
			continue
		}
		// Check if port is actually free
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("all ports %d-%d in use", pm.basePort, pm.basePort+100)
}

func (pm *PreviewManager) waitForReady(srv *DevServer) {
	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			slog.Warn("preview.dev_server.timeout", "project", srv.ProjectID, "port", srv.Port)
			return
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", srv.Port), 200*time.Millisecond)
			if err == nil {
				conn.Close()
				slog.Info("preview.dev_server.ready",
					"project", srv.ProjectID, "port", srv.Port,
					"startup_ms", time.Since(srv.StartedAt).Milliseconds())
				return
			}
		}
	}
}

// detectFramework identifies the project framework and returns (framework, startCmd).
func detectFramework(projectPath string) (string, string) {
	// Check package.json for scripts
	pkgJSON := filepath.Join(projectPath, "package.json")
	if data, err := os.ReadFile(pkgJSON); err == nil {
		content := string(data)

		// Next.js
		if fileExists(filepath.Join(projectPath, "next.config.js")) ||
			fileExists(filepath.Join(projectPath, "next.config.ts")) ||
			fileExists(filepath.Join(projectPath, "next.config.mjs")) ||
			strings.Contains(content, "\"next\"") {
			if strings.Contains(content, "\"dev\"") {
				return "nextjs", "npx next dev --port $PORT"
			}
			return "nextjs", "npx next dev --port $PORT"
		}

		// Vite (React/Vue/Svelte)
		if fileExists(filepath.Join(projectPath, "vite.config.ts")) ||
			fileExists(filepath.Join(projectPath, "vite.config.js")) ||
			strings.Contains(content, "\"vite\"") {
			return "vite", "npx vite --port $PORT --host 127.0.0.1"
		}

		// Create React App
		if strings.Contains(content, "react-scripts") {
			return "cra", "npx react-scripts start"
		}

		// Generic npm dev script
		if strings.Contains(content, "\"dev\"") {
			return "npm", "npm run dev"
		}
		if strings.Contains(content, "\"start\"") {
			return "npm", "npm start"
		}
	}

	// Go projects
	if fileExists(filepath.Join(projectPath, "go.mod")) {
		if fileExists(filepath.Join(projectPath, "main.go")) ||
			fileExists(filepath.Join(projectPath, "cmd")) {
			return "go", "go run . -port $PORT"
		}
	}

	// Python projects
	if fileExists(filepath.Join(projectPath, "manage.py")) {
		return "django", "python manage.py runserver 127.0.0.1:$PORT"
	}
	if fileExists(filepath.Join(projectPath, "app.py")) ||
		fileExists(filepath.Join(projectPath, "main.py")) {
		return "python", "python app.py"
	}

	// Ruby
	if fileExists(filepath.Join(projectPath, "Gemfile")) {
		if fileExists(filepath.Join(projectPath, "config.ru")) {
			return "rails", "bundle exec rails server -p $PORT"
		}
	}

	// Static site (has index.html)
	if fileExists(filepath.Join(projectPath, "index.html")) ||
		fileExists(filepath.Join(projectPath, "public", "index.html")) {
		return "static", "npx serve -l $PORT ."
	}

	return "", ""
}

func injectPort(cmd string, port int, framework string) string {
	portStr := fmt.Sprintf("%d", port)
	cmd = strings.ReplaceAll(cmd, "$PORT", portStr)

	// CRA uses PORT env var
	if framework == "cra" {
		return fmt.Sprintf("PORT=%s %s", portStr, cmd)
	}

	return cmd
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// writeViteHMRConfig writes a thin vite.config.preview.mjs into projectPath that
// merges the project's own Vite config with HMR overrides so the browser-side
// HMR client connects through the gateway proxy path rather than directly to the
// raw vite port.
//
// The generated file is excluded from the project via a leading comment marker
// and should be cleaned up by removeViteHMRConfig when the dev server stops.
func writeViteHMRConfig(projectPath, projectID string) error {
	// Detect the user's existing config file so we can extend it.
	base := "undefined"
	for _, name := range []string{"vite.config.ts", "vite.config.js", "vite.config.mjs", "vite.config.cjs"} {
		if fileExists(filepath.Join(projectPath, name)) {
			base = fmt.Sprintf("(await import('./%s')).default", name)
			break
		}
	}

	content := fmt.Sprintf(`// AUTO-GENERATED by Qorven preview manager — do not edit.
// This file is deleted automatically when the dev server stops.
import { mergeConfig, defineConfig } from 'vite';

const base = %s;
const override = defineConfig({
  server: {
    hmr: {
      // Route HMR websocket through the gateway preview proxy so it works
      // inside the iframe without a direct connection to the vite port.
      // clientPort:0 tells the client to use the same port as the page origin.
      clientPort: 0,
      path: '/v1/preview/%s/',
    },
  },
});

export default base && typeof base === 'object'
  ? mergeConfig(base, override)
  : override;
`, base, projectID)

	dest := filepath.Join(projectPath, "vite.config.preview.mjs")
	return os.WriteFile(dest, []byte(content), 0644)
}

// removeViteHMRConfig removes the generated vite.config.preview.mjs from the
// project directory. Called by StopDevServer.
func removeViteHMRConfig(projectPath string) {
	_ = os.Remove(filepath.Join(projectPath, "vite.config.preview.mjs"))
}
