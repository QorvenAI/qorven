// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/agent"
	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/ssestream"
	"github.com/qorvenai/qorven/internal/tools"
)


func (gw *Gateway) handleListProjects(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil { writeJSON(w, 200, []any{}); return }
	writeJSON(w, 200, gw.projectReg.List())
}

func (gw *Gateway) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil { writeJSON(w, 503, map[string]string{"error": "projects not initialized"}); return }
	var req struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"` // human-readable AI-generated name
		Path        string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name required"}); return
	}
	// Expand ~ to actual home directory
	if strings.HasPrefix(req.Path, "~/") {
		req.Path = os.Getenv("HOME") + req.Path[1:]
	}
	// Auto-generate path if not provided
	if req.Path == "" {
		home := os.Getenv("HOME")
		if home == "" { home, _ = os.UserHomeDir() }
		req.Path = home + "/qorven-workspace/" + req.Name
	}
	// Ensure workspace directory exists
	os.MkdirAll(req.Path, 0755)
	writeJSON(w, 201, gw.projectReg.Create(req.Name, req.DisplayName, req.Path))
}

func (gw *Gateway) handleGetProject(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil { writeJSON(w, 404, map[string]string{"error": "not found"}); return }
	p := gw.projectReg.Get(chi.URLParam(r, "id"))
	if p == nil { writeJSON(w, 404, map[string]string{"error": "not found"}); return }
	writeJSON(w, 200, p)
}

func (gw *Gateway) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil { writeJSON(w, 404, map[string]string{"error": "not found"}); return }
	gw.projectReg.Delete(chi.URLParam(r, "id"))
	w.WriteHeader(204)
}

func (gw *Gateway) handleAddProjectTask(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil { writeJSON(w, 503, map[string]string{"error": "not initialized"}); return }
	var req struct { Text string `json:"text"`; Priority string `json:"priority"` }
	json.NewDecoder(r.Body).Decode(&req)
	if req.Text == "" { writeJSON(w, 400, map[string]string{"error": "text required"}); return }
	t := gw.projectReg.AddTask(chi.URLParam(r, "id"), req.Text, req.Priority)
	if t == nil { writeJSON(w, 404, map[string]string{"error": "project not found"}); return }
	writeJSON(w, 201, t)
}

func (gw *Gateway) handleToggleProjectTask(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil { writeJSON(w, 503, map[string]string{"error": "not initialized"}); return }
	gw.projectReg.ToggleTask(chi.URLParam(r, "id"), chi.URLParam(r, "taskId"))
	w.WriteHeader(204)
}

func (gw *Gateway) handleUpdateProjectNotes(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil { writeJSON(w, 503, map[string]string{"error": "not initialized"}); return }
	var req struct { Notes string `json:"notes"` }
	json.NewDecoder(r.Body).Decode(&req)
	gw.projectReg.UpdateNotes(chi.URLParam(r, "id"), req.Notes)
	w.WriteHeader(204)
}

// handleBuildProject — Phase A: runs Prime's planning turn inline and
// creates the durable plan graph. Streams canonical SSE events only.
//
// The frontend subscribes to the canonical type switch (build.phase,
// plan.proposed, message.part.updated) — no legacy flat names emitted.
func (gw *Gateway) handleBuildProject(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil || gw.agentLoop == nil {
		writeJSON(w, 503, map[string]string{"error": "agent loop not initialized"})
		return
	}

	projectID := chi.URLParam(r, "id")
	project := gw.projectReg.Get(projectID)
	if project == nil {
		writeJSON(w, 404, map[string]string{"error": "project not found"})
		return
	}

	var req struct {
		Description string `json:"description"`
		Stack       string `json:"stack"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Description == "" {
		writeJSON(w, 400, map[string]string{"error": "description required"})
		return
	}

	workspace := resolveWorkspace(project)
	os.MkdirAll(workspace, 0755)

	stack := req.Stack
	if stack == "" {
		stack = "auto-detect"
	}

	gw.projectReg.UpdateBuild(projectID, func(p *tools.CodeProject) {
		p.Description = req.Description
		p.Stack = stack
		p.BuildPhase = "planning"
	})

	// SSE setup — canonical-only frames from here on.
	ew, err := ssestream.NewEmitter(w)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "streaming not supported"})
		return
	}

	_ = ew.SendEnvelope(string(apievents.TypeBuildPhase), apievents.BuildPhaseProps{
		ProjectID: projectID, Phase: "planning",
	})

	// Planning prompt — Prime must output structured JSON.
	planPrompt := fmt.Sprintf(`You are the lead architect for a software project. Analyze the description and produce a COMPLETE build plan.

Project: %s
Workspace: %s
Stack hint: %s

Description: %s

Output a JSON object (and ONLY the JSON, nothing else before or after) in this exact format:
{
  "project_name": "slug-name",
  "project_type": "standard",
  "stack": "detected stack (e.g. Next.js + TypeScript, Go + Gin, Python + FastAPI)",
  "github_repo_name": "slug-name",
  "summary": "2-3 sentence description of what will be built",
  "architecture": "brief architecture description",
  "files": ["relative/path/to/file1.ts", "relative/path/to/file2.ts"],
  "agents": [
    {"role": "frontend-dev", "tasks": ["Create React components", "Style with Tailwind"]},
    {"role": "backend-dev",  "tasks": ["Build API routes", "Database models"]},
    {"role": "devops",       "tasks": ["Write Dockerfile", "CI/CD config"]}
  ],
  "run_command": "npm run dev",
  "is_web": true
}

Set "project_type" to "qorven_app" if the user wants to build a Qorven App (a plugin that extends Qorven — has app.yaml, tools/, migrations/, frontend/bundle.js). Otherwise use "standard".

For "qorven_app" projects the files list MUST include:
- app.yaml  (see scaffold below)
- migrations/001_init.up.sql  (MUST have tenant_id UUID NOT NULL + RLS policy)
- migrations/001_init.down.sql
- tools/<name>.sh  (reads JSON from stdin, writes plain text result to stdout; shebang #!/usr/bin/env bash)
- frontend/bundle.js  (see scaffold below; NO JSX, NO imports, pure IIFE)

--- app.yaml scaffold ---
slug: <slug>
display_name: "<Name>"
version: "1.0.0"
description: "<description>"
author: ""
permissions:
  - tool_register
  - db_write
tools:
  - name: <tool_name>
    description: "<what it does>"
    command: "tools/<name>.sh"
    parameters:
      type: object
      properties:
        param1: { type: string }
      required: [param1]
frontend:
  bundle: frontend/bundle.js
  pages:
    - { id: <page-id>, label: "<Label>", icon: Package, path: <url-path> }

--- migrations/001_init.up.sql scaffold ---
CREATE TABLE IF NOT EXISTS <slug>_<table> (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE <slug>_<table> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <slug>_<table> FORCE ROW LEVEL SECURITY;
CREATE POLICY rls_<slug>_<table> ON <slug>_<table>
    USING (app_rls_bypass() OR tenant_id = app_current_tenant())
    WITH CHECK (app_rls_bypass() OR tenant_id = app_current_tenant());

--- frontend/bundle.js scaffold ---
(function() {
  const host = window.__QorvenApp;
  if (!host) return;
  const R = host.React;
  host.register({
    id: '<slug>',
    pages: [{
      id: '<page-id>',
      path: '<url-path>',
      component: function(props) {
        return R.createElement('div', { style: { padding: 24 } }, 'Hello from <Name>!');
      }
    }]
  });
})();

Be thorough in the files list. Include every file that needs to exist for the project to work.`,
		project.Name, workspace, stack, req.Description)

	var planText strings.Builder
	planRunReq := agent.RunRequest{
		AgentID:     "prime",
		SessionID:   project.SessionID + "-plan",
		UserMessage: planPrompt,
		Channel:     "code_build",
	}
	gw.agentLoop.Run(r.Context(), planRunReq, func(event agent.StreamEvent) {
		if event.Type == "text_delta" {
			planText.WriteString(event.Delta)
			_ = ew.SendEnvelope(string(apievents.TypeMessagePartUpdated), apievents.MessagePartUpdatedProps{
				MessageID: project.SessionID + "-plan",
				Kind:      "text",
				Payload:   json.RawMessage(`{"delta":` + jsonStr(event.Delta) + `}`),
			})
		}
	})

	raw := planText.String()
	planJSON := extractJSON(raw)
	var planMap map[string]any
	if err := json.Unmarshal([]byte(planJSON), &planMap); err != nil {
		_ = ew.SendEnvelope(string(apievents.TypePlanProposed), apievents.PlanProposedProps{
			ProjectID: projectID, Raw: raw,
		})
		_ = ew.SendDone()
		return
	}

	gw.projectReg.UpdateBuild(projectID, func(p *tools.CodeProject) {
		p.BuildPlan = planJSON
		p.BuildPhase = "pending_approval"
		if repo, ok := planMap["github_repo_name"].(string); ok {
			p.GitHubRepo = repo
		}
		if pt, ok := planMap["project_type"].(string); ok {
			p.ProjectType = pt
		}
	})

	// Create the durable plan graph in the DB.
	planID := ""
	if gw.plans != nil {
		actor := actorFromContext(r.Context())
		summary, _ := planMap["summary"].(string)
		dbPlan, err := gw.plans.CreatePlan(r.Context(), plans.CreatePlanInput{
			TenantID:  defaultTenant,
			ProjectID: projectID,
			SessionID: project.SessionID,
			Title:     "Build: " + project.Name,
			Summary:   summary,
			CreatedBy: actor,
			Spec:      planMap,
		})
		if err != nil {
			slog.Warn("project.build: create plan failed", "project", projectID, "err", err)
		} else {
			planID = dbPlan.ID
			plannerNode, _ := gw.plans.AppendNode(r.Context(), plans.AppendNodeInput{
				PlanID: planID, Kind: plans.KindPlanner, Title: "Planning",
				Inputs: map[string]any{
					"description": req.Description, "stack": stack,
					"agent_id": "prime", "session_id": project.SessionID + "-plan",
				},
			})
			approvalNode, _ := gw.plans.AppendNode(r.Context(), plans.AppendNodeInput{
				PlanID: planID, Kind: plans.KindHumanFeedback, Title: "Awaiting approval",
			})
			var agentNodes []*plans.Node
			if agentList, ok := planMap["agents"].([]any); ok {
				for _, a := range agentList {
					ag, ok := a.(map[string]any)
					if !ok {
						continue
					}
					role, _ := ag["role"].(string)
					if role == "" {
						continue
					}
					tasksArr, _ := ag["tasks"].([]any)
					var taskLines []string
					for _, t := range tasksArr {
						if ts, ok := t.(string); ok {
							taskLines = append(taskLines, "- "+ts)
						}
					}
					instruction := fmt.Sprintf("You are the %s agent for project %s at %s.\n\nStack: %s\n\nYour tasks:\n%s\n\nWrite complete, working code with no placeholders.",
						role, project.Name, workspace, stack, strings.Join(taskLines, "\n"))
					n, _ := gw.plans.AppendNode(r.Context(), plans.AppendNodeInput{
						PlanID: planID, Kind: plans.KindAgentTask, Title: role,
						Inputs: map[string]any{
							"agent_id": "prime", "session_id": project.SessionID + "-" + role,
							"instruction": instruction,
						},
					})
					if n != nil {
						agentNodes = append(agentNodes, n)
					}
				}
			}
			if len(agentNodes) == 0 {
				n, _ := gw.plans.AppendNode(r.Context(), plans.AppendNodeInput{
					PlanID: planID, Kind: plans.KindAgentTask, Title: "build",
					Inputs: map[string]any{
						"agent_id": "prime", "session_id": project.SessionID + "-build",
						"instruction": fmt.Sprintf("Build the complete project %q at %s. Stack: %s. Description: %s",
							project.Name, workspace, stack, req.Description),
					},
				})
				if n != nil {
					agentNodes = append(agentNodes, n)
				}
			}
			if plannerNode != nil && approvalNode != nil {
				gw.plans.AddEdge(r.Context(), planID, plannerNode.ID, approvalNode.ID, plans.CondAlways)
				for _, an := range agentNodes {
					gw.plans.AddEdge(r.Context(), planID, approvalNode.ID, an.ID, plans.CondApproved)
				}
			}
			gw.projectReg.UpdateBuild(projectID, func(p *tools.CodeProject) {
				p.PlanID = planID
			})
		}
	}

	planRaw, _ := json.Marshal(planMap)
	_ = ew.SendEnvelope(string(apievents.TypePlanProposed), apievents.PlanProposedProps{
		ProjectID: projectID,
		PlanID:    planID,
		Plan:      json.RawMessage(planRaw),
		Raw:       planJSON,
		Summary:   func() string { s, _ := planMap["summary"].(string); return s }(),
	})
	_ = ew.SendDone()
}

// handleApproveProject — Phase B: delegates to the graph runtime.
// The orchestrator drives execution; clients subscribe to handleProjectBuildStream
// for ongoing canonical SSE events (graph.node_started/completed/paused/failed).
func (gw *Gateway) handleApproveProject(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil {
		writeJSON(w, 503, map[string]string{"error": "not initialized"})
		return
	}

	projectID := chi.URLParam(r, "id")
	project := gw.projectReg.Get(projectID)
	if project == nil {
		writeJSON(w, 404, map[string]string{"error": "project not found"})
		return
	}

	planID := project.PlanID
	gw.projectReg.UpdateBuild(projectID, func(p *tools.CodeProject) {
		p.BuildPhase = "spawning"
	})

	if planID != "" && gw.orchestrator != nil {
		go func() {
			if err := gw.orchestrator.ExecutePlan(context.Background(), planID); err != nil {
				slog.Error("project.approve: ExecutePlan failed", "project", projectID, "plan", planID, "err", err)
				gw.projectReg.UpdateBuild(projectID, func(p *tools.CodeProject) {
					p.BuildPhase = "failed"
				})
				return
			}
			// Auto-install if this was a qorven_app build.
			if project.ProjectType == "qorven_app" && gw.appMgr != nil {
				if app, err := gw.appMgr.Install(context.Background(), project.Path); err != nil {
					slog.Warn("project.approve: app auto-install failed", "project", projectID, "path", project.Path, "err", err)
				} else {
					slog.Info("project.approve: app installed", "slug", app.Slug, "id", app.ID)
					if gw.events != nil {
						_ = gw.events.Emit(context.Background(), apievents.SinkAll, apievents.Type("app.installed"), map[string]any{"app_id": app.ID, "slug": app.Slug})
					}
				}
			}
		}()
	} else {
		slog.Warn("project.approve: no plan_id or orchestrator — build skipped", "project", projectID)
		gw.projectReg.UpdateBuild(projectID, func(p *tools.CodeProject) { p.BuildPhase = "failed" })
	}

	writeJSON(w, 200, map[string]string{"status": "building", "project_id": projectID, "plan_id": planID})
}

// ─── Phase B SSE stream ───────────────────────────────────────────────────────
// handleProjectBuildStream lets the frontend reconnect to get ongoing build events.

// handleProjectBuildStream serves SSE reconnects for an in-progress build.
// It attaches to gw.events so reconnecting clients receive all fine-grained
// events (agent.started, agent.progress, file.edited, etc.) in real time,
// not just coarse phase ticks from a polling loop.
func (gw *Gateway) handleProjectBuildStream(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	ew, err := ssestream.NewEmitter(w)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "streaming not supported"})
		return
	}
	// NewEmitter already flushed; grab the flusher for NewEmitterWriter below.
	flusher, _ := w.(http.Flusher)

	// Replay current build phase so reconnecting clients have a baseline.
	if gw.projectReg != nil {
		p := gw.projectReg.Get(projectID)
		if p != nil {
			_ = ew.SendEnvelope(string(apievents.TypeBuildPhase), apievents.BuildPhaseProps{
				ProjectID: projectID, Phase: p.BuildPhase, PreviewURL: p.PreviewURL,
			})
			if p.BuildPhase == "done" || p.BuildPhase == "failed" {
				_ = ew.SendDone()
				return
			}
		}
	}

	// Subscribe to the canonical events bus; the orchestrator emits
	// graph.node_* events here as execution progresses.
	reqID := fmt.Sprintf("build-stream-%s-%d", projectID, time.Now().UnixNano())
	em := ssestream.NewEmitterWriter(w, flusher)
	if gw.events != nil {
		detach := gw.events.Attach(reqID, em)
		defer detach()
	}

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := em.SendComment("heartbeat"); err != nil {
				return
			}
			if gw.projectReg != nil {
				p := gw.projectReg.Get(projectID)
				if p != nil && (p.BuildPhase == "done" || p.BuildPhase == "failed") {
					_ = ew.SendEnvelope(string(apievents.TypeBuildPhase), apievents.BuildPhaseProps{
						ProjectID: projectID, Phase: p.BuildPhase, PreviewURL: p.PreviewURL,
					})
					_ = ew.SendDone()
					return
				}
			}
		}
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func resolveWorkspace(project *tools.CodeProject) string {
	if project.Path != "" {
		return project.Path
	}
	home, _ := os.UserHomeDir()
	return home + "/qorven-workspace/" + project.Name
}

func sseSetup(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
}

// makeSender is retained for handleDaemonStream (handlers_daemon.go).
// New handlers should use ssestream.NewEmitter directly.
func makeSender(w http.ResponseWriter, flusher http.Flusher) func(string, any) {
	var mu sync.Mutex
	return func(evtType string, data any) {
		mu.Lock()
		defer mu.Unlock()

		legacy, err := json.Marshal(map[string]any{"type": evtType, "data": data})
		if err == nil {
			fmt.Fprintf(w, "data: %s\n\n", legacy)
		}

		env := apievents.Envelope{
			Type:        apievents.CanonicalType(evtType),
			EmittedAtMS: time.Now().UnixMilli(),
		}
		if err := env.Encode(data); err == nil {
			if b, err := json.Marshal(env); err == nil {
				fmt.Fprintf(w, "data: %s\n\n", b)
			}
		}

		flusher.Flush()
	}
}

// jsonStr JSON-encodes a string for embedding in a raw JSON payload literal.
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// extractJSON pulls the first JSON object from a string (agent may add prose before/after).
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return "{}"
	}
	return s[start : end+1]
}

// handleProjectTree reads the workspace directory directly and returns
// a nested file tree. No agent call — instant.
func (gw *Gateway) handleProjectTree(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil {
		writeJSON(w, 503, map[string]string{"error": "not initialized"})
		return
	}
	project := gw.projectReg.Get(chi.URLParam(r, "id"))
	if project == nil {
		writeJSON(w, 404, map[string]string{"error": "project not found"})
		return
	}
	workspace := resolveWorkspace(project)
	tree := buildDirTree(workspace, workspace, 0)
	writeJSON(w, 200, map[string]any{"tree": tree, "workspace": workspace})
}

type treeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Type     string      `json:"type"` // "file" | "dir"
	Children []treeNode  `json:"children,omitempty"`
}

func buildDirTree(root, dir string, depth int) []treeNode {
	if depth > 8 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	nodes := []treeNode{}
	// Dirs first, then files
	for _, pass := range []bool{true, false} {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == ".git" {
				continue
			}
			isDir := e.IsDir()
			if isDir != pass {
				continue
			}
			fullPath := dir + "/" + name
			node := treeNode{
				Name: name,
				Path: fullPath,
				Type: "file",
			}
			if isDir {
				node.Type = "dir"
				node.Children = buildDirTree(root, fullPath, depth+1)
			}
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// handleReadProjectFile reads a file from the project workspace directly.
func (gw *Gateway) handleReadProjectFile(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil {
		writeJSON(w, 503, map[string]string{"error": "not initialized"})
		return
	}
	project := gw.projectReg.Get(chi.URLParam(r, "id"))
	if project == nil {
		writeJSON(w, 404, map[string]string{"error": "project not found"})
		return
	}
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		writeJSON(w, 400, map[string]string{"error": "path required"})
		return
	}
	// Security: path must be inside workspace
	workspace := resolveWorkspace(project)
	if !strings.HasPrefix(filePath, workspace) {
		writeJSON(w, 403, map[string]string{"error": "path outside workspace"})
		return
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "file not found"})
		return
	}
	writeJSON(w, 200, map[string]any{
		"path":    filePath,
		"content": string(data),
		"size":    len(data),
	})
}

// handleWriteProjectFile writes a file to the project workspace directly.
func (gw *Gateway) handleWriteProjectFile(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil {
		writeJSON(w, 503, map[string]string{"error": "not initialized"})
		return
	}
	project := gw.projectReg.Get(chi.URLParam(r, "id"))
	if project == nil {
		writeJSON(w, 404, map[string]string{"error": "project not found"})
		return
	}
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeJSON(w, 400, map[string]string{"error": "path and content required"})
		return
	}
	workspace := resolveWorkspace(project)
	if !strings.HasPrefix(req.Path, workspace) {
		// Allow relative paths by prepending workspace
		if !strings.HasPrefix(req.Path, "/") {
			req.Path = workspace + "/" + req.Path
		} else {
			writeJSON(w, 403, map[string]string{"error": "path outside workspace"})
			return
		}
	}
	os.MkdirAll(strings.TrimSuffix(req.Path, "/"+last(req.Path)), 0755)
	if err := os.WriteFile(req.Path, []byte(req.Content), 0644); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"path": req.Path, "size": len(req.Content)})
}

func last(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

// Ensure unused imports don't break compilation
var _ = providers.Message{}
