// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build windows

package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var jobStore = &bgJobStore{jobs: make(map[string]*bgJob)}

type bgJob struct {
	id       string
	cmd      *exec.Cmd
	output   strings.Builder
	mu       sync.Mutex
	done     bool
	exitCode int
	exitErr  error
	started  time.Time
	desc     string
}

type bgJobStore struct {
	mu   sync.Mutex
	jobs map[string]*bgJob
}

func (s *bgJobStore) add(j *bgJob) {
	s.mu.Lock()
	s.jobs[j.id] = j
	s.mu.Unlock()
}

func (s *bgJobStore) get(id string) (*bgJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	return j, ok
}

func (s *bgJobStore) remove(id string) {
	s.mu.Lock()
	delete(s.jobs, id)
	s.mu.Unlock()
}

type JobSpawnTool struct{ workspace string }

func NewJobSpawnTool(workspace string) *JobSpawnTool { return &JobSpawnTool{workspace: workspace} }
func (t *JobSpawnTool) Name() string                 { return "job_spawn" }
func (t *JobSpawnTool) Description() string {
	return "Start a long-running background job and return a job ID."
}
func (t *JobSpawnTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"command": map[string]any{"type": "string"}},
		"required":   []string{"command"},
	}
}

func (t *JobSpawnTool) Execute(_ context.Context, args map[string]any) *Result {
	command, _ := args["command"].(string)
	if command == "" {
		return ErrorResult("command is required")
	}
	id := uuid.NewString()
	desc, _ := args["description"].(string)
	j := &bgJob{id: id, started: time.Now(), desc: desc}
	cmd := exec.Command("cmd", "/C", command)
	cmd.Dir = t.workspace
	cmd.Env = os.Environ()
	j.cmd = cmd

	pr, pw, err := os.Pipe()
	if err != nil {
		return ErrorResult("failed to create pipe: " + err.Error())
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return ErrorResult(fmt.Sprintf("spawn failed: %v", err))
	}
	pw.Close()
	jobStore.add(j)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				j.mu.Lock()
				j.output.Write(buf[:n])
				j.mu.Unlock()
			}
			if err != nil {
				break
			}
		}
		pr.Close()
		exitErr := cmd.Wait()
		j.mu.Lock()
		j.done = true
		j.exitErr = exitErr
		j.mu.Unlock()
	}()

	return &Result{ForLLM: fmt.Sprintf("job_id: %s\nPID: %d", id, cmd.Process.Pid)}
}

type JobOutputTool struct{}

func NewJobOutputTool() *JobOutputTool { return &JobOutputTool{} }
func (t *JobOutputTool) Name() string  { return "job_output" }
func (t *JobOutputTool) Description() string {
	return "Get current output from a background job."
}
func (t *JobOutputTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"job_id": map[string]any{"type": "string"}},
		"required":   []string{"job_id"},
	}
}

func (t *JobOutputTool) Execute(_ context.Context, args map[string]any) *Result {
	id, _ := args["job_id"].(string)
	j, ok := jobStore.get(id)
	if !ok {
		return ErrorResult("job not found: " + id)
	}
	j.mu.Lock()
	out := j.output.String()
	done := j.done
	exitErr := j.exitErr
	j.mu.Unlock()

	status := "running"
	if done {
		if exitErr != nil {
			status = "failed: " + exitErr.Error()
		} else {
			status = "done"
		}
	}
	return &Result{ForLLM: fmt.Sprintf("status: %s\n\n%s", status, out)}
}

type JobKillTool struct{}

func NewJobKillTool() *JobKillTool { return &JobKillTool{} }
func (t *JobKillTool) Name() string { return "job_kill" }
func (t *JobKillTool) Description() string {
	return "Kill a running background job."
}
func (t *JobKillTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"job_id": map[string]any{"type": "string"}},
		"required":   []string{"job_id"},
	}
}

func (t *JobKillTool) Execute(_ context.Context, args map[string]any) *Result {
	id, _ := args["job_id"].(string)
	j, ok := jobStore.get(id)
	if !ok {
		return ErrorResult("job not found: " + id)
	}
	if j.cmd != nil && j.cmd.Process != nil {
		j.cmd.Process.Kill()
	}
	jobStore.remove(id)
	return &Result{ForLLM: "job " + id + " killed"}
}

func (t *JobKillTool) ListJobs() []*bgJob {
	jobStore.mu.Lock()
	defer jobStore.mu.Unlock()
	out := make([]*bgJob, 0, len(jobStore.jobs))
	for _, j := range jobStore.jobs {
		out = append(out, j)
	}
	return out
}

type JobListTool struct{}

func NewJobListTool() *JobListTool { return &JobListTool{} }
func (t *JobListTool) Name() string { return "job_list" }
func (t *JobListTool) Description() string {
	return "List all background jobs started with job_spawn, showing their status and elapsed time."
}
func (t *JobListTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *JobListTool) Execute(_ context.Context, _ map[string]any) *Result {
	jobStore.mu.Lock()
	jobs := make([]*bgJob, 0, len(jobStore.jobs))
	for _, j := range jobStore.jobs {
		jobs = append(jobs, j)
	}
	jobStore.mu.Unlock()
	if len(jobs) == 0 {
		return TextResult("No background jobs running.")
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d background job(s):\n\n", len(jobs)))
	for _, j := range jobs {
		j.mu.Lock()
		status := "running"
		if j.done {
			status = fmt.Sprintf("exited(%d)", j.exitCode)
		}
		label := j.desc
		if label == "" {
			label = "(no description)"
		}
		elapsed := time.Since(j.started).Round(time.Second)
		j.mu.Unlock()
		sb.WriteString(fmt.Sprintf("  %s  %-10s  %s  (%s)\n", j.id, status, label, elapsed))
	}
	return TextResult(sb.String())
}

