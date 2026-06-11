// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/gateway/deploy"
	"github.com/qorvenai/qorven/internal/tools"
)

// DeployStatus tracks the state of a deployment.
type DeployStatus string

const (
	DeployPending   DeployStatus = "pending"
	DeployBuilding  DeployStatus = "building"
	DeployPushing   DeployStatus = "pushing"
	DeployLive      DeployStatus = "live"
	DeployFailed    DeployStatus = "failed"
	DeployStopped   DeployStatus = "stopped"
)

// Deployment represents a single deploy attempt for a project.
type Deployment struct {
	ID          string       `json:"id"`
	ProjectID   string       `json:"project_id"`
	ProjectName string       `json:"project_name"`
	Status      DeployStatus `json:"status"`
	Framework   string       `json:"framework"`
	URL         string       `json:"url,omitempty"`
	Dockerfile  string       `json:"dockerfile,omitempty"`
	Error       string       `json:"error,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	BuildLog    []string     `json:"build_log,omitempty"`
	// Added by migration 047: deploy lineage + target.
	ReleaseID   string `json:"release_id,omitempty"`  // release_gates.id that triggered this deploy
	Target      string `json:"target,omitempty"`      // "local" | "hosted" | "cloud:*"
	DeployedURL string `json:"deployed_url,omitempty"` // canonical public URL once live
}

// DeployManager manages project deployments with DB persistence.
type DeployManager struct {
	mu          sync.RWMutex
	deployments map[string]*Deployment // deployID → deployment
	byProject   map[string]string      // projectID → latest deployID
	db          *pgxpool.Pool
}

func NewDeployManager() *DeployManager {
	return &DeployManager{
		deployments: make(map[string]*Deployment),
		byProject:   make(map[string]string),
	}
}

func (dm *DeployManager) SetDB(pool *pgxpool.Pool) {
	dm.db = pool
	dm.loadFromDB()
}

func (dm *DeployManager) Get(id string) *Deployment {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.deployments[id]
}

func (dm *DeployManager) GetByProject(projectID string) *Deployment {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if did, ok := dm.byProject[projectID]; ok {
		return dm.deployments[did]
	}
	return nil
}

func (dm *DeployManager) Create(d *Deployment) {
	dm.mu.Lock()
	dm.deployments[d.ID] = d
	dm.byProject[d.ProjectID] = d.ID
	dm.mu.Unlock()
	dm.persistCreate(d)
}

func (dm *DeployManager) Update(id string, fn func(*Deployment)) {
	dm.mu.Lock()
	d, ok := dm.deployments[id]
	if ok {
		fn(d)
		d.UpdatedAt = time.Now()
	}
	dm.mu.Unlock()
	if ok {
		dm.persistUpdate(d)
	}
}

func (dm *DeployManager) persistCreate(d *Deployment) {
	if dm.db == nil {
		return
	}
	go func() {
		// release_id is nullable uuid — pass nil when empty so pgx doesn't fail.
		var releaseID *string
		if d.ReleaseID != "" {
			releaseID = &d.ReleaseID
		}
		target := d.Target
		if target == "" {
			target = "local"
		}
		_, err := dm.db.Exec(context.Background(), `
			INSERT INTO deployments
				(id, project_id, project_name, status, framework, url, dockerfile, error,
				 build_log, created_at, updated_at, release_id, target, deployed_url)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::uuid, $13, $14)
			ON CONFLICT (id) DO UPDATE
				SET status = EXCLUDED.status, updated_at = EXCLUDED.updated_at
		`, d.ID, d.ProjectID, d.ProjectName, string(d.Status), d.Framework, d.URL,
			d.Dockerfile, d.Error, d.BuildLog, d.CreatedAt, d.UpdatedAt,
			releaseID, target, d.DeployedURL)
		if err != nil {
			slog.Debug("deploy.persist_create", "err", err)
		}
	}()
}

func (dm *DeployManager) persistUpdate(d *Deployment) {
	if dm.db == nil {
		return
	}
	go func() {
		dm.mu.RLock()
		status := string(d.Status)
		url := d.URL
		errStr := d.Error
		buildLog := d.BuildLog
		updatedAt := d.UpdatedAt
		deployedURL := d.DeployedURL
		dm.mu.RUnlock()

		_, err := dm.db.Exec(context.Background(), `
			UPDATE deployments
			SET status = $2, url = $3, error = $4, build_log = $5, updated_at = $6,
			    deployed_url = $7
			WHERE id = $1::uuid
		`, d.ID, status, url, errStr, buildLog, updatedAt, deployedURL)
		if err != nil {
			slog.Debug("deploy.persist_update", "err", err)
		}
	}()
}

func (dm *DeployManager) loadFromDB() {
	if dm.db == nil {
		return
	}
	rows, err := dm.db.Query(context.Background(), `
		SELECT DISTINCT ON (project_id)
			id, project_id, project_name, status, framework, url, dockerfile, error,
			build_log, created_at, updated_at,
			COALESCE(release_id::text, ''), COALESCE(target, ''), COALESCE(deployed_url, '')
		FROM deployments
		ORDER BY project_id, created_at DESC
	`)
	if err != nil {
		slog.Debug("deploy.load_from_db", "err", err)
		return
	}
	defer rows.Close()

	dm.mu.Lock()
	defer dm.mu.Unlock()
	for rows.Next() {
		var d Deployment
		var status string
		if err := rows.Scan(
			&d.ID, &d.ProjectID, &d.ProjectName, &status, &d.Framework,
			&d.URL, &d.Dockerfile, &d.Error, &d.BuildLog, &d.CreatedAt, &d.UpdatedAt,
			&d.ReleaseID, &d.Target, &d.DeployedURL,
		); err != nil {
			continue
		}
		d.Status = DeployStatus(status)
		dm.deployments[d.ID] = &d
		dm.byProject[d.ProjectID] = d.ID
	}
	if len(dm.deployments) > 0 {
		slog.Info("deploy.loaded_from_db", "count", len(dm.deployments))
	}
}

// deployRequest is the optional JSON body for POST /v1/projects/:id/deploy.
type deployRequest struct {
	Target     string `json:"target"`      // "local" | "hosted" (default) | "cloud:vercel" | "cloud:netlify"
	ReleaseTag string `json:"release_tag"` // git ref to deploy (cloud targets); empty → "main"
}

// startDeploy creates and launches a deployment for the given project.
// releaseID may be empty (manual deploy via API); when non-empty it is stamped
// on the deployments row so the deployment is traceable to its release gate.
// Returns the new Deployment record (status=building) immediately; the actual
// deploy runs in a detached goroutine.
func (gw *Gateway) startDeploy(project *tools.CodeProject, target, releaseID, releaseTag string) *Deployment {
	projectID := project.ID
	projectName := project.Name
	projectPath := project.Path
	briefID := project.InceptionBriefID

	// Detect framework (used as a hint by targets that write their own Dockerfile).
	framework, _ := detectFramework(projectPath)
	if framework == "" {
		framework = "static"
	}
	dockerfile := generateDockerfile(framework, projectPath)
	slug := sanitizeSlug(projectName)

	// Build the deploy spec from project metadata.
	spec := deploy.Spec{
		ProjectID:   projectID,
		ProjectPath: projectPath,
		Slug:        slug,
		Framework:   framework,
		ReleaseTag:  releaseTag,
		RepoOwner:   project.GitHubOwner,
		RepoName:    project.GitHubRepo,
	}

	// Create deployment record — status building from the start.
	rec := &Deployment{
		ID:          uuid.New().String(),
		ProjectID:   projectID,
		ProjectName: projectName,
		Status:      DeployBuilding,
		Framework:   framework,
		Dockerfile:  dockerfile,
		Target:      target,
		ReleaseID:   releaseID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	gw.deployMgr.Create(rec)

	// Track as a Command Center job.
	if gw.commandCenter != nil {
		now := time.Now()
		gw.commandCenter.AddJob(&AgentJob{
			ID:        rec.ID,
			ProjectID: projectID,
			AgentID:   "deploy",
			AgentName: "Deploy Agent",
			Title:     fmt.Sprintf("Deploy %s (%s)", projectName, target),
			Status:    JobStatusRunning,
			StartedAt: &now,
		})
	}

	// Launch the deploy asynchronously — use a detached context so the
	// goroutine is not cancelled when the HTTP request (or calling function) returns.
	go func() {
		gw.emitProjectEvent(context.Background(), briefID, "deploy_started",
			fmt.Sprintf("Deploy %s started (%s)", projectName, target),
			map[string]any{"project_id": projectID, "target": target, "deploy_id": rec.ID, "release_id": releaseID},
			"", "")

		res, err := gw.deployReg.Deploy(context.Background(), target, spec)
		if err != nil {
			errMsg := err.Error()
			gw.deployMgr.Update(rec.ID, func(d *Deployment) {
				d.Status = DeployFailed
				d.Error = errMsg
			})
			if gw.commandCenter != nil {
				now := time.Now()
				gw.commandCenter.UpdateJob(rec.ID, func(j *AgentJob) {
					j.Status = JobStatusFailed
					j.Error = errMsg
					j.CompletedAt = &now
				})
			}
			gw.emitProjectEvent(context.Background(), briefID, "deploy_failed",
				fmt.Sprintf("Deploy %s failed: %s", projectName, errMsg),
				map[string]any{"project_id": projectID, "target": target, "deploy_id": rec.ID, "error": errMsg, "release_id": releaseID},
				"", "")
			if briefID != "" {
				gw.triggerFixLoop(context.Background(), briefID,
					"deploy", "deploy-"+rec.ID,
					"Deploy failed: "+rec.ProjectName,
					errMsg)
			}
			slog.Error("deploy.failed", "project", projectID, "target", target, "err", err)
			return
		}

		// Success — mark live with the REAL url from the target.
		gw.deployMgr.Update(rec.ID, func(d *Deployment) {
			d.Status = DeployLive
			d.URL = res.URL
			d.DeployedURL = res.URL
		})
		if gw.commandCenter != nil {
			now := time.Now()
			gw.commandCenter.UpdateJob(rec.ID, func(j *AgentJob) {
				j.Status = JobStatusCompleted
				j.Progress = 100
				j.CompletedAt = &now
				if j.StartedAt != nil {
					j.DurationMs = now.Sub(*j.StartedAt).Milliseconds()
				}
			})
		}

		// Reflect deployed URL on the project's PreviewURL (best-effort).
		if gw.projectReg != nil && res.URL != "" {
			gw.projectReg.UpdateBuild(projectID, func(p *tools.CodeProject) {
				p.PreviewURL = res.URL
			})
		}

		gw.emitProjectEvent(context.Background(), briefID, "deploy_live",
			fmt.Sprintf("Deploy %s live at %s", projectName, res.URL),
			map[string]any{"project_id": projectID, "target": res.Target, "deploy_id": rec.ID, "url": res.URL, "release_id": releaseID},
			"", "")
		slog.Info("deploy.complete", "project", projectID, "target", res.Target, "url", res.URL, "release_id", releaseID)
	}()

	return rec
}

// handleDeploy starts a deployment for a project, dispatching to the chosen
// deploy target via the registry.
func (gw *Gateway) handleDeploy(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing project id"})
		return
	}

	if gw.projectReg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not initialized"})
		return
	}

	project := gw.projectReg.Get(projectID)
	if project == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	// Parse optional body — tolerate missing/empty body.
	var req deployRequest
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	target := req.Target
	if target == "" {
		target = "hosted"
	}

	if gw.deployReg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "deploy registry not initialised (database required)"})
		return
	}

	// Delegate to shared helper; manual deploys carry no release_id.
	rec := gw.startDeploy(project, target, "", req.ReleaseTag)
	writeJSON(w, http.StatusAccepted, rec)
}

// handleDeployStatus returns the status of the latest deployment for a project.
func (gw *Gateway) handleDeployStatus(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	deploy := gw.deployMgr.GetByProject(projectID)
	if deploy == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "none",
		})
		return
	}
	writeJSON(w, http.StatusOK, deploy)
}

// handleDeployStop stops a running deployment.
// For local and hosted targets both use a Docker container named after the
// project slug (local: <slug>, hosted: qorven-hosted-<slug>). We stop both
// possible container names so the caller does not need to track which target
// was used. Status is set to stopped regardless of docker exit code.
func (gw *Gateway) handleDeployStop(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	dep := gw.deployMgr.GetByProject(projectID)
	if dep == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no deployment found"})
		return
	}

	// Derive the container name(s) from the project name.
	slug := sanitizeSlug(dep.ProjectName)

	// Stop and remove both the local-target container (<slug>) and the
	// hosted-target container (qorven-hosted-<slug>). Both errors are ignored
	// — the container may already be gone, or Docker may be absent entirely.
	execCommand("docker", "rm", "-f", slug).Run()                          //nolint:errcheck
	execCommand("docker", "rm", "-f", "qorven-hosted-"+slug).Run()         //nolint:errcheck

	gw.deployMgr.Update(dep.ID, func(d *Deployment) {
		d.Status = DeployStopped
	})

	if gw.commandCenter != nil {
		now := time.Now()
		gw.commandCenter.UpdateJob(dep.ID, func(j *AgentJob) {
			j.Status = JobStatusCancelled
			j.CompletedAt = &now
		})
	}

	slog.Info("deploy.stopped", "project", projectID, "slug", slug)
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// handleDeployDockerfile returns the generated Dockerfile for a project without deploying.
func (gw *Gateway) handleDeployDockerfile(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	if gw.projectReg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not initialized"})
		return
	}

	project := gw.projectReg.Get(projectID)
	if project == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	projectPath := project.Path

	framework, _ := detectFramework(projectPath)
	if framework == "" {
		framework = "static"
	}

	dockerfile := generateDockerfile(framework, projectPath)

	writeJSON(w, http.StatusOK, map[string]string{
		"framework":  framework,
		"dockerfile": dockerfile,
	})
}


func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func allocateDeployPort(slug string) int {
	// Deterministic port from slug hash to keep consistent across redeploys
	h := 0
	for _, c := range slug {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return 9200 + (h % 800) // ports 9200-9999
}

func waitForPort(port int, timeout time.Duration) bool {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			return false
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
			if err == nil {
				conn.Close()
				return true
			}
		}
	}
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// handleDeployList returns all deployments (for admin/dashboard).
func (gw *Gateway) handleDeployList(w http.ResponseWriter, r *http.Request) {
	gw.deployMgr.mu.RLock()
	result := make([]*Deployment, 0, len(gw.deployMgr.deployments))
	for _, d := range gw.deployMgr.deployments {
		result = append(result, d)
	}
	gw.deployMgr.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"deployments": result})
}

// --- Dockerfile generation ---

func generateDockerfile(framework, projectPath string) string {
	switch framework {
	case "nextjs":
		return dockerfileNextJS()
	case "vite":
		return dockerfileVite()
	case "cra":
		return dockerfileCRA()
	case "go":
		return dockerfileGo(projectPath)
	case "django":
		return dockerfileDjango()
	case "python":
		return dockerfilePython()
	case "rails":
		return dockerfileRails()
	default:
		return dockerfileStatic()
	}
}

func dockerfileNextJS() string {
	return `FROM node:20-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* pnpm-lock.yaml* ./
RUN if [ -f pnpm-lock.yaml ]; then corepack enable && pnpm install --frozen-lockfile; \
    elif [ -f package-lock.json ]; then npm ci; \
    else npm install; fi

FROM node:20-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
COPY --from=builder /app/public ./public
EXPOSE 3000
CMD ["node", "server.js"]
`
}

func dockerfileVite() string {
	return `FROM node:20-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* pnpm-lock.yaml* ./
RUN if [ -f pnpm-lock.yaml ]; then corepack enable && pnpm install --frozen-lockfile; \
    elif [ -f package-lock.json ]; then npm ci; \
    else npm install; fi
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY <<'EOF' /etc/nginx/conf.d/default.conf
server {
    listen 80;
    root /usr/share/nginx/html;
    location / {
        try_files $uri $uri/ /index.html;
    }
}
EOF
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`
}

func dockerfileCRA() string {
	return `FROM node:20-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=build /app/build /usr/share/nginx/html
COPY <<'EOF' /etc/nginx/conf.d/default.conf
server {
    listen 80;
    root /usr/share/nginx/html;
    location / {
        try_files $uri $uri/ /index.html;
    }
}
EOF
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`
}

func dockerfileGo(projectPath string) string {
	modName := "app"
	modFile := filepath.Join(projectPath, "go.mod")
	if data, err := os.ReadFile(modFile); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "module ") {
				modName = strings.TrimSpace(strings.TrimPrefix(line, "module "))
				break
			}
		}
	}
	_ = modName
	return `FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /server /server
EXPOSE 8080
CMD ["/server"]
`
}

func dockerfileDjango() string {
	return `FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
RUN python manage.py collectstatic --noinput 2>/dev/null || true
EXPOSE 8000
CMD ["gunicorn", "--bind", "0.0.0.0:8000", "--workers", "2", "config.wsgi:application"]
`
}

func dockerfilePython() string {
	return `FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt* ./
RUN if [ -f requirements.txt ]; then pip install --no-cache-dir -r requirements.txt; fi
COPY . .
EXPOSE 8000
CMD ["python", "app.py"]
`
}

func dockerfileRails() string {
	return `FROM ruby:3.3-slim
WORKDIR /app
RUN apt-get update -qq && apt-get install -y build-essential libpq-dev nodejs
COPY Gemfile Gemfile.lock ./
RUN bundle install --without development test
COPY . .
RUN bundle exec rails assets:precompile 2>/dev/null || true
EXPOSE 3000
CMD ["bundle", "exec", "rails", "server", "-b", "0.0.0.0", "-p", "3000"]
`
}

func dockerfileStatic() string {
	return `FROM nginx:alpine
COPY . /usr/share/nginx/html
COPY <<'EOF' /etc/nginx/conf.d/default.conf
server {
    listen 80;
    root /usr/share/nginx/html;
    location / {
        try_files $uri $uri/ /index.html;
    }
}
EOF
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`
}

func sanitizeSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, slug)
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "app"
	}
	return slug
}

// handleDeployLogs returns streaming deploy logs for the given deployment
func (gw *Gateway) handleDeployLogs(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	deploy := gw.deployMgr.GetByProject(projectID)
	if deploy == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no deployment found"})
		return
	}

	type logResp struct {
		ID       string       `json:"id"`
		Status   DeployStatus `json:"status"`
		URL      string       `json:"url,omitempty"`
		Error    string       `json:"error,omitempty"`
		BuildLog []string     `json:"build_log"`
	}

	gw.deployMgr.mu.RLock()
	resp := logResp{
		ID:       deploy.ID,
		Status:   deploy.Status,
		URL:      deploy.URL,
		Error:    deploy.Error,
		BuildLog: deploy.BuildLog,
	}
	gw.deployMgr.mu.RUnlock()

	writeJSON(w, http.StatusOK, resp)
}

