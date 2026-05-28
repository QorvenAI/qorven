// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
}

// DeployManager manages project deployments.
type DeployManager struct {
	mu          sync.RWMutex
	deployments map[string]*Deployment // deployID → deployment
	byProject   map[string]string      // projectID → latest deployID
}

func NewDeployManager() *DeployManager {
	return &DeployManager{
		deployments: make(map[string]*Deployment),
		byProject:   make(map[string]string),
	}
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
}

func (dm *DeployManager) Update(id string, fn func(*Deployment)) {
	dm.mu.Lock()
	if d, ok := dm.deployments[id]; ok {
		fn(d)
		d.UpdatedAt = time.Now()
	}
	dm.mu.Unlock()
}

// handleDeploy starts a deployment for a project.
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

	projectPath := project.Path
	projectName := project.Name

	// Detect framework and generate Dockerfile
	framework, _ := detectFramework(projectPath)
	if framework == "" {
		framework = "static"
	}

	dockerfile := generateDockerfile(framework, projectPath)

	// Create deployment record
	deploy := &Deployment{
		ID:          uuid.New().String(),
		ProjectID:   projectID,
		ProjectName: projectName,
		Status:      DeployBuilding,
		Framework:   framework,
		Dockerfile:  dockerfile,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	gw.deployMgr.Create(deploy)

	// Write the Dockerfile to the project
	dockerfilePath := filepath.Join(projectPath, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
		slog.Error("deploy.write_dockerfile", "err", err)
	}

	// Track as a Command Center job
	if gw.commandCenter != nil {
		now := time.Now()
		gw.commandCenter.AddJob(&AgentJob{
			ID:        deploy.ID,
			ProjectID: projectID,
			AgentID:   "deploy",
			AgentName: "Deploy Agent",
			Title:     fmt.Sprintf("Deploy %s to qorven.run", projectName),
			Status:    JobStatusRunning,
			StartedAt: &now,
		})
	}

	// Simulate deploy process (in production this would trigger actual container build)
	go gw.runDeploy(deploy, projectPath)

	writeJSON(w, http.StatusAccepted, deploy)
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
func (gw *Gateway) handleDeployStop(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	deploy := gw.deployMgr.GetByProject(projectID)
	if deploy == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no deployment found"})
		return
	}

	gw.deployMgr.Update(deploy.ID, func(d *Deployment) {
		d.Status = DeployStopped
	})

	if gw.commandCenter != nil {
		now := time.Now()
		gw.commandCenter.UpdateJob(deploy.ID, func(j *AgentJob) {
			j.Status = JobStatusCancelled
			j.CompletedAt = &now
		})
	}

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

// runDeploy simulates the deploy pipeline stages.
func (gw *Gateway) runDeploy(deploy *Deployment, projectPath string) {
	slug := sanitizeSlug(deploy.ProjectName)
	deployURL := fmt.Sprintf("https://%s.qorven.run", slug)

	appendLog := func(msg string) {
		gw.deployMgr.Update(deploy.ID, func(d *Deployment) {
			d.BuildLog = append(d.BuildLog, msg)
		})
	}

	// Stage 1: Building container image
	appendLog("Generating Dockerfile...")
	time.Sleep(800 * time.Millisecond)
	appendLog(fmt.Sprintf("Detected framework: %s", deploy.Framework))

	// Stage 2: Install dependencies
	gw.deployMgr.Update(deploy.ID, func(d *Deployment) { d.Status = DeployBuilding })
	appendLog("Installing dependencies...")
	time.Sleep(1200 * time.Millisecond)
	appendLog("Dependencies installed")

	// Stage 3: Build
	appendLog("Building production bundle...")
	time.Sleep(1500 * time.Millisecond)
	appendLog("Build complete")

	// Stage 4: Push to registry
	gw.deployMgr.Update(deploy.ID, func(d *Deployment) { d.Status = DeployPushing })
	appendLog("Pushing container image...")
	time.Sleep(1000 * time.Millisecond)
	appendLog("Image pushed to registry")

	// Stage 5: Deploy to edge
	appendLog("Deploying to edge network...")
	time.Sleep(800 * time.Millisecond)
	appendLog(fmt.Sprintf("Live at %s", deployURL))

	// Mark as live
	gw.deployMgr.Update(deploy.ID, func(d *Deployment) {
		d.Status = DeployLive
		d.URL = deployURL
	})

	if gw.commandCenter != nil {
		now := time.Now()
		gw.commandCenter.UpdateJob(deploy.ID, func(j *AgentJob) {
			j.Status = JobStatusCompleted
			j.Progress = 100
			j.CompletedAt = &now
			if j.StartedAt != nil {
				j.DurationMs = now.Sub(*j.StartedAt).Milliseconds()
			}
		})
	}

	slog.Info("deploy.complete", "project", deploy.ProjectID, "url", deployURL)
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

