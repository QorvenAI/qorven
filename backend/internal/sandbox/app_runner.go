// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunAppParams holds everything needed to start a new sandboxed app container.
type RunAppParams struct {
	TenantID    string
	SessionID   string
	AgentID     string
	ImageOrRepo string
	Port        int
	Label       string
	TTLMinutes  int
	Env         map[string]string
	BaseURL     string
}

// RunningApp is the runtime record of a started app container.
type RunningApp struct {
	ID           string
	TenantID     string
	SessionID    string
	AgentID      string
	ContainerID  string
	Image        string
	Label        string
	ProxyPrefix  string
	InternalPort int
	HostPort     int
	Status       string
	Env          map[string]string
	ExpiresAt    time.Time
	CreatedAt    time.Time
	ProxyURL     string
}

// AppRunner manages the lifecycle of agent-spawned app containers and their
// reverse-proxy route table. It is safe for concurrent use.
type AppRunner struct {
	db      *pgxpool.Pool
	baseURL string
	routes  sync.Map
	stopCh  chan struct{}
	buildMu sync.Mutex
}

// NewAppRunner creates an AppRunner, recovers in-memory routes from DB, and
// starts the TTL reaper goroutine. Call StopAll(ctx) on gateway shutdown.
func NewAppRunner(pool *pgxpool.Pool, baseURL string) *AppRunner {
	ar := &AppRunner{
		db:      pool,
		baseURL: strings.TrimRight(baseURL, "/"),
		stopCh:  make(chan struct{}),
	}
	if err := ar.RecoverRoutes(context.Background()); err != nil {
		slog.Warn("app_runner.recover_routes_failed", "err", err)
	}
	go ar.reaperLoop()
	return ar
}

// Start pulls (or builds) the image and launches the container.
func (ar *AppRunner) Start(ctx context.Context, p RunAppParams) (*RunningApp, error) {
	if p.Port <= 0 || p.Port > 65535 {
		return nil, fmt.Errorf("run_app: invalid port %d (must be 1-65535)", p.Port)
	}

	isGitURL := strings.HasPrefix(p.ImageOrRepo, "https://") || strings.HasPrefix(p.ImageOrRepo, "git@")
	if !isGitURL {
		// Validate Docker image name: [registry/]name[:tag][@digest]
		// Must not start with - (flag injection) and must not contain shell metacharacters
		if strings.HasPrefix(p.ImageOrRepo, "-") || strings.ContainsAny(p.ImageOrRepo, " \t\n;&|`$(){}\\") {
			return nil, fmt.Errorf("run_app: invalid image name %q", p.ImageOrRepo)
		}
	}

	// Also check env values
	for k, v := range p.Env {
		if strings.Contains(v, "--network") {
			return nil, fmt.Errorf("env var %s: network override is not permitted", k)
		}
	}

	ttl := p.TTLMinutes
	if ttl <= 0 {
		ttl = 30
	}
	if ttl > 480 {
		ttl = 480
	}

	label := p.Label
	if label == "" {
		label = labelFromImageOrRepo(p.ImageOrRepo)
	}

	image, err := ar.resolveImage(ctx, p.ImageOrRepo)
	if err != nil {
		return nil, fmt.Errorf("run_app: resolve image: %w", err)
	}

	prefix := uuid.New().String()
	containerName := "qorven-app-" + prefix[:8]

	args := []string{
		"run", "-d",
		"--name", containerName,
		"--label", "qorven.app=true",
		"--label", "qorven.prefix=" + prefix,
		"--network", "bridge",
		"--security-opt", "no-new-privileges",
		"--memory", "1g",
		"--cpus", "1.0",
		"-p", fmt.Sprintf("0:%d", p.Port),
	}
	for k, v := range p.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, image)

	slog.Info("app_runner.docker_run", "name", containerName, "image", image, "port", p.Port)

	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run_app: docker run: %w — %s", err, strings.TrimSpace(string(out)))
	}
	containerID := strings.TrimSpace(string(out))
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}

	hostPort, err := ar.getHostPort(ctx, containerID, p.Port)
	if err != nil {
		exec.Command("docker", "rm", "-f", containerID).Run() //nolint:errcheck
		return nil, fmt.Errorf("run_app: get host port: %w", err)
	}

	target := fmt.Sprintf("http://127.0.0.1:%d", hostPort)
	expiresAt := time.Now().Add(time.Duration(ttl) * time.Minute)

	envJSON, _ := json.Marshal(p.Env)

	app := &RunningApp{}
	err = ar.db.QueryRow(ctx,
		`INSERT INTO running_apps
			(tenant_id, session_id, agent_id, container_id, image, label, proxy_prefix,
			 internal_port, host_port, status, env, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'running',$10,$11)
		 RETURNING id, tenant_id, COALESCE(session_id::text,''), COALESCE(agent_id::text,''),
		           container_id, image, label, proxy_prefix, internal_port, host_port,
		           status, env, expires_at, created_at`,
		nilUUID(p.TenantID), nilUUID(p.SessionID), nilUUID(p.AgentID),
		containerID, image, label, prefix,
		p.Port, hostPort,
		envJSON, expiresAt,
	).Scan(
		&app.ID, &app.TenantID, &app.SessionID, &app.AgentID,
		&app.ContainerID, &app.Image, &app.Label, &app.ProxyPrefix,
		&app.InternalPort, &app.HostPort,
		&app.Status, &envJSON, &app.ExpiresAt, &app.CreatedAt,
	)
	if err != nil {
		exec.Command("docker", "rm", "-f", containerID).Run() //nolint:errcheck
		return nil, fmt.Errorf("run_app: db insert: %w", err)
	}
	json.Unmarshal(envJSON, &app.Env) //nolint:errcheck
	app.ProxyURL = ar.baseURL + "/sandbox/" + prefix + "/"

	ar.routes.Store(prefix, target)
	slog.Info("app_runner.started", "id", app.ID, "label", label, "prefix", prefix, "host_port", hostPort)
	return app, nil
}

// Stop kills a container and marks it stopped in DB.
// If tenantID is non-empty the query is filtered to that tenant (API path).
// If tenantID is empty the filter is skipped (agent tool path, already tenant-scoped).
func (ar *AppRunner) Stop(ctx context.Context, id, tenantID string) error {
	var containerID, prefix string
	var err error
	if tenantID != "" {
		err = ar.db.QueryRow(ctx,
			`SELECT container_id, proxy_prefix FROM running_apps WHERE id=$1 AND tenant_id=$2 AND status='running'`,
			id, tenantID,
		).Scan(&containerID, &prefix)
	} else {
		err = ar.db.QueryRow(ctx,
			`SELECT container_id, proxy_prefix FROM running_apps WHERE id=$1 AND status='running'`,
			id,
		).Scan(&containerID, &prefix)
	}
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("app not found")
		}
		return fmt.Errorf("stop_app: db select: %w", err)
	}

	if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", containerID).CombinedOutput(); err != nil {
		slog.Warn("app_runner.stop.rm_failed", "id", containerID, "err", err, "out", string(out))
	}

	if _, err := ar.db.Exec(ctx,
		`UPDATE running_apps SET status='stopped' WHERE id=$1`, id,
	); err != nil {
		return fmt.Errorf("stop_app: db update: %w", err)
	}

	ar.routes.Delete(prefix)
	slog.Info("app_runner.stopped", "id", id, "container", containerID)
	return nil
}

// List returns running apps for a tenant.
func (ar *AppRunner) List(ctx context.Context, tenantID string) ([]RunningApp, error) {
	rows, err := ar.db.Query(ctx,
		`SELECT id, tenant_id, COALESCE(session_id::text,''), COALESCE(agent_id::text,''),
		        container_id, image, label, proxy_prefix, internal_port, host_port,
		        status, env, expires_at, created_at
		 FROM running_apps
		 WHERE tenant_id=$1 AND status='running'
		 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []RunningApp
	for rows.Next() {
		var a RunningApp
		var envJSON []byte
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.SessionID, &a.AgentID,
			&a.ContainerID, &a.Image, &a.Label, &a.ProxyPrefix,
			&a.InternalPort, &a.HostPort,
			&a.Status, &envJSON, &a.ExpiresAt, &a.CreatedAt,
		); err != nil {
			slog.Warn("app_runner: list scan failed", "err", err)
			continue
		}
		json.Unmarshal(envJSON, &a.Env) //nolint:errcheck
		a.ProxyURL = ar.baseURL + "/sandbox/" + a.ProxyPrefix + "/"
		apps = append(apps, a)
	}
	return apps, nil
}

// Prune kills all containers whose expires_at has passed. Returns count removed.
func (ar *AppRunner) Prune(ctx context.Context) int {
	rows, err := ar.db.Query(ctx,
		`UPDATE running_apps SET status='stopped'
		 WHERE status='running' AND expires_at < now()
		 RETURNING container_id, proxy_prefix`)
	if err != nil {
		slog.Warn("app_runner.prune.query_failed", "err", err)
		return 0
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var containerID, prefix string
		rows.Scan(&containerID, &prefix) //nolint:errcheck
		ar.routes.Delete(prefix)
		if err := exec.CommandContext(ctx, "docker", "rm", "-f", containerID).Run(); err != nil {
			slog.Warn("app_runner.prune.rm_failed", "id", containerID, "err", err)
		}
		count++
	}
	if count > 0 {
		slog.Info("app_runner.pruned", "count", count)
	}
	return count
}

// StopAll removes all running containers. Call on gateway shutdown.
func (ar *AppRunner) StopAll(ctx context.Context) error {
	// Signal reaper to stop
	select {
	case <-ar.stopCh:
		// already closed
	default:
		close(ar.stopCh)
	}

	rows, err := ar.db.Query(ctx,
		`UPDATE running_apps SET status='stopped' WHERE status='running'
		 RETURNING container_id, proxy_prefix`)
	if err != nil {
		return fmt.Errorf("app_runner.stop_all: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var containerID, prefix string
		rows.Scan(&containerID, &prefix) //nolint:errcheck
		ar.routes.Delete(prefix)
		exec.CommandContext(ctx, "docker", "rm", "-f", containerID).Run() //nolint:errcheck
	}
	return nil
}

// RecoverRoutes re-hydrates the in-memory route table from DB on startup.
func (ar *AppRunner) RecoverRoutes(ctx context.Context) error {
	rows, err := ar.db.Query(ctx,
		`SELECT proxy_prefix, host_port FROM running_apps
		 WHERE status='running' AND expires_at > now()`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var prefix string
		var hostPort int
		rows.Scan(&prefix, &hostPort) //nolint:errcheck
		target := fmt.Sprintf("http://127.0.0.1:%d", hostPort)
		ar.routes.Store(prefix, target)
		count++
	}
	slog.Info("app_runner.routes_recovered", "count", count)
	return nil
}

// ProxyHandler returns an http.Handler for /sandbox/{prefix}/{path...}.
func (ar *AppRunner) ProxyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := chi.URLParam(r, "prefix")
		if prefix == "" {
			// fallback for safety
			stripped := strings.TrimPrefix(r.URL.Path, "/sandbox/")
			parts := strings.SplitN(stripped, "/", 2)
			prefix = parts[0]
		}

		val, ok := ar.routes.Load(prefix)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"app not found or expired"}`)) //nolint:errcheck
			return
		}
		targetStr := val.(string)
		targetURL, err := url.Parse(targetStr)
		if err != nil {
			http.Error(w, "proxy misconfiguration", http.StatusInternalServerError)
			return
		}

		stripped := strings.TrimPrefix(r.URL.Path, "/sandbox/")
		parts := strings.SplitN(stripped, "/", 2)
		subPath := "/"
		if len(parts) > 1 && parts[1] != "" {
			subPath = "/" + parts[1]
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = subPath
		r2.URL.RawPath = ""

		for _, h := range []string{"X-Forwarded-Host", "X-Real-Ip", "X-Forwarded-For"} {
			r2.Header.Del(h)
		}
		r2.Header.Del("Authorization")
		r2.Header.Del("Cookie")
		r2.Header.Set("X-Forwarded-Prefix", "/sandbox/"+prefix)
		r2.Header.Set("X-Forwarded-Proto", "https")
		r2.Host = targetURL.Host

		// WebSocket upgrade: tunnel raw TCP instead of using the reverse proxy.
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			targetConn, err := net.Dial("tcp", targetURL.Host)
			if err != nil {
				http.Error(w, "bad gateway", http.StatusBadGateway)
				return
			}
			defer targetConn.Close()
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "websocket not supported", http.StatusInternalServerError)
				return
			}
			clientConn, bufrw, err := hj.Hijack()
			if err != nil {
				http.Error(w, "hijack failed", http.StatusInternalServerError)
				return
			}
			defer clientConn.Close()
			// Flush any bytes the HTTP server already buffered from the client
			if bufrw.Reader.Buffered() > 0 {
				if _, copyErr := io.CopyN(targetConn, bufrw.Reader, int64(bufrw.Reader.Buffered())); copyErr != nil {
					return
				}
			}
			if err := r2.Write(targetConn); err != nil {
				return
			}
			go io.Copy(targetConn, clientConn)
			io.Copy(clientConn, targetConn)
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.ModifyResponse = func(resp *http.Response) error {
			resp.Header.Del("Authorization")
			resp.Header.Del("Cookie")
			resp.Header.Del("Set-Cookie")
			return nil
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("app_runner.proxy_error", "prefix", prefix, "target", targetStr, "err", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error":"container not responding — it may still be starting up"}`)) //nolint:errcheck
		}
		proxy.ServeHTTP(w, r2)
	})
}

// --- internal helpers ---

func (ar *AppRunner) reaperLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ar.stopCh:
			return
		case <-ticker.C:
			ar.Prune(context.Background())
		}
	}
}

func (ar *AppRunner) resolveImage(ctx context.Context, imageOrRepo string) (string, error) {
	if strings.HasPrefix(imageOrRepo, "https://") || strings.HasPrefix(imageOrRepo, "git@") {
		return ar.buildFromGit(ctx, imageOrRepo)
	}
	if out, err := exec.CommandContext(ctx, "docker", "pull", imageOrRepo).CombinedOutput(); err != nil {
		return "", fmt.Errorf("docker pull %s: %w — %s", imageOrRepo, err, strings.TrimSpace(string(out)))
	}
	return imageOrRepo, nil
}

func (ar *AppRunner) buildFromGit(ctx context.Context, repoURL string) (string, error) {
	ar.buildMu.Lock()
	defer ar.buildMu.Unlock()

	tag := "qorven-app-" + uuid.New().String()[:8]
	buildDir := filepath.Join("/tmp", "qorven-appbuild", tag)
	defer os.RemoveAll(buildDir)

	cloneOut, err := exec.CommandContext(ctx, "git", "clone", "--depth", "1", repoURL, buildDir).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone %s: %w — %s", repoURL, err, strings.TrimSpace(string(cloneOut)))
	}

	if _, err := os.Stat(filepath.Join(buildDir, "Dockerfile")); os.IsNotExist(err) {
		return "", fmt.Errorf("no Dockerfile found in repository root")
	}

	var buildOut bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "build", "-t", tag, buildDir)
	cmd.Stdout = &buildOut
	cmd.Stderr = &buildOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker build: %w\n%s", err, buildOut.String())
	}
	return tag, nil
}

func (ar *AppRunner) getHostPort(ctx context.Context, containerID string, internalPort int) (int, error) {
	portSpec := fmt.Sprintf("%d/tcp", internalPort)
	format := fmt.Sprintf(`{{(index (index .NetworkSettings.Ports "%s") 0).HostPort}}`, portSpec)
	out, err := exec.CommandContext(ctx, "docker", "inspect",
		"--format", format, containerID).Output()
	if err != nil {
		return 0, fmt.Errorf("docker inspect: %w", err)
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("parse host port %q: %w", strings.TrimSpace(string(out)), err)
	}
	return port, nil
}

func labelFromImageOrRepo(s string) string {
	if strings.Contains(s, "://") || strings.HasPrefix(s, "git@") {
		parts := strings.Split(strings.TrimSuffix(s, ".git"), "/")
		return parts[len(parts)-1]
	}
	s = strings.SplitN(s, ":", 2)[0]
	parts := strings.Split(s, "/")
	return parts[len(parts)-1]
}

func nilUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}
