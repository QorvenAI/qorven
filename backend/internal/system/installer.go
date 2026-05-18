// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package system

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
)

// Installer manages background installation of local models.
type Installer struct {
	mu       sync.Mutex
	jobs     map[string]*InstallJob
}

type InstallJob struct {
	Package  string `json:"package"`
	Status   string `json:"status"` // pending, downloading, installing, running, done, error
	Progress int    `json:"progress"` // 0-100
	Error    string `json:"error,omitempty"`
	Method   string `json:"method"` // pip, docker
}

func NewInstaller() *Installer {
	return &Installer{jobs: make(map[string]*InstallJob)}
}

// Install starts a background installation.
func (inst *Installer) Install(ctx context.Context, pkg string) error {
	inst.mu.Lock()
	if j, ok := inst.jobs[pkg]; ok && (j.Status == "downloading" || j.Status == "installing") {
		inst.mu.Unlock()
		return fmt.Errorf("%s is already installing", pkg)
	}
	job := &InstallJob{Package: pkg, Status: "pending", Method: "pip"}
	inst.jobs[pkg] = job
	inst.mu.Unlock()

	go inst.run(ctx, job)
	return nil
}

// Status returns current job status.
func (inst *Installer) Status(pkg string) *InstallJob {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.jobs[pkg]
}

func (inst *Installer) run(ctx context.Context, job *InstallJob) {
	inst.setStatus(job, "downloading", 10)

	var cmds [][]string
	switch job.Package {
	case "kokoro":
		cmds = [][]string{
			{"pip3", "install", "kokoro", "soundfile"},
		}
	case "whisper-tiny":
		cmds = [][]string{
			{"pip3", "install", "insanely-fast-whisper"},
		}
		job.Method = "pip"
	case "whisper-base":
		cmds = [][]string{
			{"pip3", "install", "insanely-fast-whisper"},
		}
	case "whisper-small":
		cmds = [][]string{
			{"pip3", "install", "insanely-fast-whisper"},
		}
	case "whisper-medium":
		cmds = [][]string{
			{"pip3", "install", "insanely-fast-whisper"},
		}
	case "whisper-large-v3":
		cmds = [][]string{
			{"pip3", "install", "insanely-fast-whisper"},
		}
	case "edge-tts":
		cmds = [][]string{
			{"pip3", "install", "edge-tts"},
		}
	case "silero-vad":
		// ONNX model — just download the file
		cmds = [][]string{
			{"pip3", "install", "onnxruntime"},
		}
	default:
		inst.setError(job, fmt.Sprintf("unknown package: %s", job.Package))
		return
	}

	inst.setStatus(job, "installing", 40)
	for _, cmd := range cmds {
		slog.Info("system.install", "cmd", cmd)
		c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
		out, err := c.CombinedOutput()
		if err != nil {
			inst.setError(job, fmt.Sprintf("%s: %s", err, string(out)))
			return
		}
	}

	inst.setStatus(job, "done", 100)
	slog.Info("system.install.done", "package", job.Package)
}

func (inst *Installer) setStatus(job *InstallJob, status string, progress int) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	job.Status = status
	job.Progress = progress
}

func (inst *Installer) setError(job *InstallJob, err string) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	job.Status = "error"
	job.Error = err
	slog.Error("system.install.error", "package", job.Package, "error", err)
}
