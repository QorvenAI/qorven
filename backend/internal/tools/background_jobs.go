// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !windows

package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// jobStore is the process-lifetime registry of background jobs.
// It is package-level so SpawnTool and OutputTool share state without
// needing a shared struct passed through the registry.
var jobStore = &bgJobStore{jobs: make(map[string]*bgJob)}

type bgJob struct {
	id       string
	cmd      *exec.Cmd
	buf      bytes.Buffer
	mu       sync.Mutex
	done     bool
	exitCode int
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
	j, ok := s.jobs[id]
	s.mu.Unlock()
	return j, ok
}

func (s *bgJobStore) remove(id string) {
	s.mu.Lock()
	delete(s.jobs, id)
	s.mu.Unlock()
}

// ── job_spawn ─────────────────────────────────────────────────────────────────

// JobSpawnTool starts a long-running command in the background and returns
// a job ID that can be used with job_output and job_kill.
type JobSpawnTool struct {
	workspace string
}

func NewJobSpawnTool(workspace string) *JobSpawnTool { return &JobSpawnTool{workspace: workspace} }
func (t *JobSpawnTool) Name() string                 { return "job_spawn" }
func (t *JobSpawnTool) Description() string {
	return "Start a long-running command in the background (e.g. dev server, test runner, build watcher). Returns a job ID immediately. Use job_output to read its output and job_kill to stop it."
}
func (t *JobSpawnTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to run in the background",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Short label for this job (e.g. 'dev server', 'test watcher')",
			},
			"dir": map[string]any{
				"type":        "string",
				"description": "Working directory. Defaults to the workspace root.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *JobSpawnTool) Execute(ctx context.Context, args map[string]any) *Result {
	command, _ := args["command"].(string)
	if command == "" {
		return ErrorResult("command is required")
	}
	desc, _ := args["description"].(string)
	dir, _ := args["dir"].(string)
	if dir == "" {
		dir = WorkspaceFromCtx(ctx)
		if dir == "" {
			dir = t.workspace
		}
	}

	job := &bgJob{
		id:      uuid.New().String()[:8],
		started: time.Now(),
		desc:    desc,
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	// New session so SIGTERM reaches the whole process group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	pr, pw, err := os.Pipe()
	if err != nil {
		return ErrorResult("failed to create pipe: " + err.Error())
	}
	cmd.Stdout = pw
	cmd.Stderr = pw
	job.cmd = cmd

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return ErrorResult("failed to start job: " + err.Error())
	}
	pw.Close() // write end belongs to child now

	jobStore.add(job)
	registerBackgroundProcess(cmd.Process.Pid, desc)

	// Goroutine: stream output into job.buf
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				job.mu.Lock()
				// Cap buffer at 512KB
				if job.buf.Len() < 512*1024 {
					job.buf.Write(buf[:n])
				}
				job.mu.Unlock()
			}
			if err != nil {
				break
			}
		}
		pr.Close()
	}()

	// Goroutine: reap process
	go func() {
		err := cmd.Wait()
		unregisterBackgroundProcess(cmd.Process.Pid)
		job.mu.Lock()
		job.done = true
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				job.exitCode = ee.ExitCode()
			} else {
				job.exitCode = -1
			}
		}
		job.mu.Unlock()
	}()

	label := desc
	if label == "" {
		label = command
	}
	return TextResult(fmt.Sprintf("Job started. ID: %s\nCommand: %s\nPID: %d\n\nUse job_output %s to read output.", job.id, label, cmd.Process.Pid, job.id))
}

// ── job_output ────────────────────────────────────────────────────────────────

// JobOutputTool reads the buffered output of a background job.
type JobOutputTool struct{}

func NewJobOutputTool() *JobOutputTool { return &JobOutputTool{} }
func (t *JobOutputTool) Name() string  { return "job_output" }
func (t *JobOutputTool) Description() string {
	return "Read the buffered output of a background job started with job_spawn. Returns what the job has printed so far."
}
func (t *JobOutputTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"job_id": map[string]any{
				"type":        "string",
				"description": "Job ID returned by job_spawn",
			},
			"tail": map[string]any{
				"type":        "integer",
				"description": "Return only the last N bytes of output (default: all)",
			},
		},
		"required": []string{"job_id"},
	}
}

func (t *JobOutputTool) Execute(_ context.Context, args map[string]any) *Result {
	jobID, _ := args["job_id"].(string)
	if jobID == "" {
		return ErrorResult("job_id is required")
	}
	job, ok := jobStore.get(jobID)
	if !ok {
		return ErrorResult(fmt.Sprintf("job %s not found — it may have been killed or never started", jobID))
	}

	job.mu.Lock()
	out := job.buf.String()
	done := job.done
	exitCode := job.exitCode
	elapsed := time.Since(job.started).Round(time.Second)
	job.mu.Unlock()

	if tailN, ok := args["tail"].(float64); ok && tailN > 0 && int(tailN) < len(out) {
		out = out[len(out)-int(tailN):]
	}

	status := "running"
	if done {
		if exitCode == 0 {
			status = "exited (code 0)"
		} else {
			status = fmt.Sprintf("exited (code %d)", exitCode)
		}
	}

	if out == "" {
		out = "(no output yet)"
	}
	header := fmt.Sprintf("Job %s — %s — elapsed %s\n---\n", jobID, status, elapsed)
	if len(out) > 20000 {
		out = "[... truncated ...]\n" + out[len(out)-20000:]
	}
	return TextResult(header + out)
}

// ── job_kill ──────────────────────────────────────────────────────────────────

// JobKillTool terminates a background job.
type JobKillTool struct{}

func NewJobKillTool() *JobKillTool { return &JobKillTool{} }
func (t *JobKillTool) Name() string { return "job_kill" }
func (t *JobKillTool) Description() string {
	return "Terminate a background job started with job_spawn. Sends SIGTERM to the process group."
}
func (t *JobKillTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"job_id": map[string]any{
				"type":        "string",
				"description": "Job ID returned by job_spawn",
			},
		},
		"required": []string{"job_id"},
	}
}

func (t *JobKillTool) Execute(_ context.Context, args map[string]any) *Result {
	jobID, _ := args["job_id"].(string)
	if jobID == "" {
		return ErrorResult("job_id is required")
	}
	job, ok := jobStore.get(jobID)
	if !ok {
		return ErrorResult(fmt.Sprintf("job %s not found", jobID))
	}

	job.mu.Lock()
	done := job.done
	job.mu.Unlock()

	if done {
		jobStore.remove(jobID)
		return TextResult(fmt.Sprintf("Job %s already exited.", jobID))
	}

	if job.cmd != nil && job.cmd.Process != nil {
		// Signal the whole process group.
		_ = syscall.Kill(-job.cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(300 * time.Millisecond)
		_ = syscall.Kill(-job.cmd.Process.Pid, syscall.SIGKILL)
	}
	jobStore.remove(jobID)
	return TextResult(fmt.Sprintf("Job %s terminated.", jobID))
}

// ── job_list ──────────────────────────────────────────────────────────────────

// JobListTool lists all running background jobs.
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
