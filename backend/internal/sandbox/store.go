// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type Run struct {
	ID         string    `json:"id"`
	AgentID    string    `json:"agent_id"`
	Command    string    `json:"command"`
	Language   string    `json:"language,omitempty"`
	Code       string    `json:"code,omitempty"`
	ExitCode   int       `json:"exit_code"`
	Output     string    `json:"output"`
	DurationMs int       `json:"duration_ms"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// Execute runs a command and stores the result.
func (s *Store) Execute(ctx context.Context, agentID, command, language, code string) (*Run, error) {
	workspace := filepath.Join("/tmp/qorven-workspace", agentID)
	os.MkdirAll(workspace, 0755)

	// If code provided, write to temp file
	if code != "" && language != "" {
		ext := map[string]string{"python": ".py", "javascript": ".js", "go": ".go", "bash": ".sh", "typescript": ".ts"}
		e := ext[language]
		if e == "" {
			e = ".txt"
		}
		tmpFile := filepath.Join(workspace, "run"+e)
		os.WriteFile(tmpFile, []byte(code), 0644)
		runner := map[string]string{"python": "python3", "javascript": "node", "bash": "bash", "go": "go run"}
		r := runner[language]
		if r == "" {
			r = "bash"
		}
		command = r + " " + tmpFile
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = workspace
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start).Milliseconds()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}
	// Truncate output
	if len(output) > 50000 {
		output = output[:50000] + "\n... (truncated)"
	}

	status := "completed"
	if exitCode != 0 {
		status = "failed"
	}

	run := &Run{}
	storeErr := s.pool.QueryRow(ctx,
		`INSERT INTO sandbox_runs (agent_id, command, language, code, exit_code, output, duration_ms, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, agent_id, command, COALESCE(language,''), COALESCE(code,''), exit_code, output, duration_ms, status, created_at`,
		agentID, command, language, code, exitCode, output, int(duration), status,
	).Scan(&run.ID, &run.AgentID, &run.Command, &run.Language, &run.Code, &run.ExitCode, &run.Output, &run.DurationMs, &run.Status, &run.CreatedAt)
	if storeErr != nil {
		return nil, storeErr
	}
	return run, nil
}

func (s *Store) ListRuns(ctx context.Context, agentID string, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, command, COALESCE(language,''), COALESCE(code,''), exit_code, COALESCE(output,''), duration_ms, status, created_at
		 FROM sandbox_runs WHERE agent_id = $1 ORDER BY created_at DESC LIMIT $2`, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var r Run
		rows.Scan(&r.ID, &r.AgentID, &r.Command, &r.Language, &r.Code, &r.ExitCode, &r.Output, &r.DurationMs, &r.Status, &r.CreatedAt)
		runs = append(runs, r)
	}
	return runs, nil
}

func (s *Store) GetRun(ctx context.Context, id string) (*Run, error) {
	r := &Run{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, agent_id, command, COALESCE(language,''), COALESCE(code,''), exit_code, COALESCE(output,''), duration_ms, status, created_at
		 FROM sandbox_runs WHERE id = $1`, id,
	).Scan(&r.ID, &r.AgentID, &r.Command, &r.Language, &r.Code, &r.ExitCode, &r.Output, &r.DurationMs, &r.Status, &r.CreatedAt)
	return r, err
}

// ListArtifacts returns files in the agent's workspace.
func (s *Store) ListArtifacts(ctx context.Context, agentID string) ([]map[string]any, error) {
	workspace := filepath.Join("/tmp/qorven-workspace", agentID)
	artifacts := []map[string]any{}
	filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(workspace, path)
		artifacts = append(artifacts, map[string]any{
			"name": info.Name(), "path": rel, "size": info.Size(), "modified": info.ModTime(),
		})
		return nil
	})
	return artifacts, nil
}

func (s *Store) ArtifactPath(agentID, path string) string {
	return filepath.Join("/tmp/qorven-workspace", agentID, filepath.Clean(path))
}

func (s *Store) ValidateArtifactPath(agentID, path string) error {
	full := s.ArtifactPath(agentID, path)
	base := filepath.Join("/tmp/qorven-workspace", agentID)
	if !filepath.HasPrefix(full, base) {
		return fmt.Errorf("path traversal denied")
	}
	return nil
}
