// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/crypto"
)

// CustomToolDef is a runtime-defined tool stored in the database.
type CustomToolDef struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Command     string         `json:"command"`     // shell template: "dig +short {{.record_type}} {{.domain}}"
	Timeout     int            `json:"timeout_seconds"`
	Env         map[string]string `json:"env,omitempty"` // encrypted in DB
	AgentID     *string        `json:"agent_id,omitempty"` // nil = global
	Enabled     bool           `json:"enabled"`
}

// DynamicTool executes a shell command template with arguments.
type DynamicTool struct {
	def       CustomToolDef
	workspace string
}

func NewDynamicTool(def CustomToolDef, workspace string) *DynamicTool {
	return &DynamicTool{def: def, workspace: workspace}
}

func (t *DynamicTool) Name() string        { return t.def.Name }
func (t *DynamicTool) Description() string { return t.def.Description }
func (t *DynamicTool) Parameters() map[string]any { return t.def.Parameters }

func (t *DynamicTool) Execute(ctx context.Context, args map[string]any) *Result {
	// Shell-escape all arguments (defense against injection)
	safeArgs := make(map[string]any, len(args))
	for k, v := range args {
		if s, ok := v.(string); ok {
			safeArgs[k] = shellEscape(s)
		} else {
			safeArgs[k] = v
		}
	}

	// Render command template
	tmpl, err := template.New("cmd").Parse(t.def.Command)
	if err != nil { return ErrorResult(fmt.Sprintf("invalid command template: %v", err)) }

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, safeArgs); err != nil {
		return ErrorResult(fmt.Sprintf("template error: %v", err))
	}
	command := buf.String()

	// Security check
	if denied, pattern := IsShellDenied(command); denied {
		return ErrorResult(fmt.Sprintf("command blocked: %s", pattern))
	}

	timeout := time.Duration(t.def.Timeout) * time.Second
	if timeout <= 0 { timeout = 60 * time.Second }
	if timeout > 5*time.Minute { timeout = 5 * time.Minute }

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, SafeShell, "-c", command)
	cmd.Dir = t.workspace
	env := safeEnv(t.workspace)
	for k, v := range t.def.Env { env = append(env, k+"="+v) }
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	result := string(out)
	if len(result) > maxExecOutput { result = result[:maxExecOutput] + "\n[truncated]" }

	if err != nil {
		return ErrorResult(fmt.Sprintf("exit error: %v\n%s", err, result))
	}
	if result == "" { result = "(no output)" }
	return TextResult(result)
}

// shellEscape escapes a string for safe shell interpolation.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// --- Custom Tool Store (PostgreSQL) ---

type CustomToolStore struct {
	pool   *pgxpool.Pool
	encKey string
}

func NewCustomToolStore(pool *pgxpool.Pool, encKey string) *CustomToolStore {
	return &CustomToolStore{pool: pool, encKey: encKey}
}

func (s *CustomToolStore) Create(ctx context.Context, tenantID string, def CustomToolDef) (string, error) {
	if !IsSafeSlug(def.Name) {
		return "", fmt.Errorf("invalid tool name: must be lowercase alphanumeric with hyphens")
	}
	params, _ := json.Marshal(def.Parameters)
	encEnv := []byte{}
	if len(def.Env) > 0 && s.encKey != "" {
		envJSON, _ := json.Marshal(def.Env)
		encEnv, _ = crypto.Encrypt(envJSON, s.encKey)
	}
	timeout := def.Timeout
	if timeout <= 0 { timeout = 60 }

	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO custom_tools (tenant_id, name, description, parameters, command, timeout_seconds, env, agent_id, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true) RETURNING id`,
		tenantID, def.Name, def.Description, params, def.Command, timeout, encEnv, def.AgentID,
	).Scan(&id)
	return id, err
}

func (s *CustomToolStore) List(ctx context.Context, tenantID string) ([]CustomToolDef, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, parameters, command, timeout_seconds, agent_id, enabled
		 FROM custom_tools WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()

	tools := []CustomToolDef{}
	for rows.Next() {
		var def CustomToolDef
		params := []byte{}
		rows.Scan(&def.ID, &def.Name, &def.Description, &params, &def.Command, &def.Timeout, &def.AgentID, &def.Enabled)
		json.Unmarshal(params, &def.Parameters)
		tools = append(tools, def)
	}
	return tools, nil
}

func (s *CustomToolStore) Delete(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM custom_tools WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	return err
}

// LoadAndRegister loads custom tools from DB and registers them in the registry.
func (s *CustomToolStore) LoadAndRegister(ctx context.Context, tenantID, workspace string, reg *Registry) error {
	tools, err := s.List(ctx, tenantID)
	if err != nil { return err }
	for _, def := range tools {
		if !def.Enabled { continue }
		// Decrypt env if needed
		reg.Register(NewDynamicTool(def, workspace))
	}
	return nil
}
