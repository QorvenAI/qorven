// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// RunAppParams mirrors sandbox.RunAppParams. The field-for-field copy avoids
// an import cycle: the sandbox package does not import tools, and the tools
// package does not import sandbox. The gateway wires them together.
type RunAppParams struct {
	TenantID    string
	SessionID   string
	AgentID     string
	ImageOrRepo string
	Port        int
	Label       string
	TTLMinutes  int
	Env         map[string]string
}

// RunningAppResult mirrors the fields of sandbox.RunningApp that are relevant
// to tool output. The gateway maps sandbox.RunningApp → RunningAppResult.
type RunningAppResult struct {
	ID          string
	ContainerID string
	Image       string
	Label       string
	ProxyPrefix string
	ProxyURL    string
	Status      string
	HostPort    int
	ExpiresAt   time.Time
}

// Function-type aliases injected at construction time. Using function values
// instead of a direct sandbox.AppRunner reference breaks the import cycle.
type RunAppFunc func(ctx context.Context, params RunAppParams) (*RunningAppResult, error)
type ListAppsFunc func(ctx context.Context, tenantID string) ([]RunningAppResult, error)
type StopAppFunc func(ctx context.Context, tenantID, id string) error

// ---------------------------------------------------------------------------
// RunAppTool — Tool 1
// ---------------------------------------------------------------------------

// RunAppTool starts a sandboxed Docker container and returns its proxy URL.
type RunAppTool struct {
	runApp RunAppFunc
}

// NewRunAppTool returns a RunAppTool backed by the provided function.
func NewRunAppTool(fn RunAppFunc) *RunAppTool {
	return &RunAppTool{runApp: fn}
}

func (t *RunAppTool) Name() string { return "run_app" }

func (t *RunAppTool) Description() string {
	return "Start a sandboxed Docker container from a Docker Hub image or a git repository URL. " +
		"Returns a proxy URL the agent can share with the user to access the running app. " +
		"The container is automatically stopped after the TTL expires (default 30 minutes, max 480)."
}

func (t *RunAppTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"image_or_repo": map[string]any{
				"type":        "string",
				"description": "Docker Hub image (e.g. \"n8nio/n8n\", \"postgres:16\") or git repository URL starting with https:// or git@. The repository must have a Dockerfile at its root.",
			},
			"port": map[string]any{
				"type":        "integer",
				"description": "Port the container listens on internally (e.g. 5678 for n8n, 5432 for postgres, 8080 for most web apps).",
			},
			"label": map[string]any{
				"type":        "string",
				"description": "Human-readable display name for the app. Defaults to the image/repo name when omitted.",
			},
			"ttl_minutes": map[string]any{
				"type":        "integer",
				"description": "How long to keep the container running, in minutes. Range: 1–480. Defaults to 30.",
			},
			"env": map[string]any{
				"type":                 "object",
				"description":          "Environment variables to inject into the container as key-value pairs.",
				"additionalProperties": map[string]any{"type": "string"},
			},
		},
		"required": []string{"image_or_repo", "port"},
	}
}

func (t *RunAppTool) Execute(ctx context.Context, args map[string]any) *Result {
	imageOrRepo, _ := args["image_or_repo"].(string)
	imageOrRepo = strings.TrimSpace(imageOrRepo)
	if imageOrRepo == "" {
		return ErrorResult("image_or_repo is required")
	}

	port, ok := toInt(args["port"])
	if !ok || port <= 0 || port > 65535 {
		return ErrorResult("port must be an integer between 1 and 65535")
	}

	label, _ := args["label"].(string)
	label = strings.TrimSpace(label)

	ttl := 30
	if n, ok := toInt(args["ttl_minutes"]); ok {
		if n < 1 {
			n = 1
		}
		if n > 480 {
			n = 480
		}
		ttl = n
	}

	var env map[string]string
	if raw, ok := args["env"].(map[string]any); ok && len(raw) > 0 {
		env = make(map[string]string, len(raw))
		for k, v := range raw {
			switch sv := v.(type) {
			case string:
				env[k] = sv
			case float64:
				env[k] = strconv.FormatFloat(sv, 'f', -1, 64)
			case bool:
				env[k] = strconv.FormatBool(sv)
			default:
				return ErrorResult(fmt.Sprintf("env[%s]: value must be a string", k))
			}
		}
	}

	params := RunAppParams{
		TenantID:    TenantIDFromCtx(ctx),
		SessionID:   SessionIDFromCtx(ctx),
		AgentID:     AgentIDFromCtx(ctx),
		ImageOrRepo: imageOrRepo,
		Port:        port,
		Label:       label,
		TTLMinutes:  ttl,
		Env:         env,
	}

	app, err := t.runApp(ctx, params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("run_app failed: %v", err))
	}

	displayLabel := app.Label
	if displayLabel == "" {
		displayLabel = imageOrRepo
	}

	return TextResult(fmt.Sprintf(
		"App started: %s\nURL: %s\nTTL: %d minutes\nContainer: %s",
		displayLabel, app.ProxyURL, ttl, app.ContainerID,
	))
}

// ---------------------------------------------------------------------------
// ListRunningAppsTool — Tool 2
// ---------------------------------------------------------------------------

// ListRunningAppsTool lists all running sandboxed apps for the current tenant.
type ListRunningAppsTool struct {
	listApps ListAppsFunc
}

// NewListRunningAppsTool returns a ListRunningAppsTool backed by the provided function.
func NewListRunningAppsTool(fn ListAppsFunc) *ListRunningAppsTool {
	return &ListRunningAppsTool{listApps: fn}
}

func (t *ListRunningAppsTool) Name() string { return "list_running_apps" }

func (t *ListRunningAppsTool) Description() string {
	return "List all sandboxed app containers currently running for this workspace. " +
		"Returns each app's label, container ID, proxy URL, and remaining TTL."
}

func (t *ListRunningAppsTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *ListRunningAppsTool) Execute(ctx context.Context, args map[string]any) *Result {
	tenantID := TenantIDFromCtx(ctx)

	apps, err := t.listApps(ctx, tenantID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("list_running_apps failed: %v", err))
	}

	if len(apps) == 0 {
		return TextResult("No apps running.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Running apps (%d):\n", len(apps)))
	for _, app := range apps {
		minsLeft := int(math.Round(time.Until(app.ExpiresAt).Minutes()))
		if minsLeft < 0 {
			minsLeft = 0
		}
		lbl := app.Label
		if lbl == "" {
			lbl = app.Image
		}
		sb.WriteString(fmt.Sprintf("- %s  [%s]  %s  expires in %dm\n",
			lbl, app.ContainerID, app.ProxyURL, minsLeft))
	}

	return TextResult(strings.TrimRight(sb.String(), "\n"))
}

// ---------------------------------------------------------------------------
// StopAppTool — Tool 3
// ---------------------------------------------------------------------------

// StopAppTool stops a running sandboxed app container by its ID.
type StopAppTool struct {
	stopApp StopAppFunc
}

// NewStopAppTool returns a StopAppTool backed by the provided function.
func NewStopAppTool(fn StopAppFunc) *StopAppTool {
	return &StopAppTool{stopApp: fn}
}

func (t *StopAppTool) Name() string { return "stop_app" }

func (t *StopAppTool) Description() string {
	return "Stop a running sandboxed app container. " +
		"Use list_running_apps to find the app ID. The container is removed immediately."
}

func (t *StopAppTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "The app ID to stop. Obtain it from list_running_apps.",
			},
		},
		"required": []string{"id"},
	}
}

func (t *StopAppTool) Execute(ctx context.Context, args map[string]any) *Result {
	id, _ := args["id"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrorResult("id is required")
	}

	tenantID := TenantIDFromCtx(ctx)
	if err := t.stopApp(ctx, tenantID, id); err != nil {
		return ErrorResult(fmt.Sprintf("stop_app failed: %v", err))
	}

	return TextResult("App stopped.")
}
