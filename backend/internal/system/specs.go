// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package system

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Specs holds detected system hardware info.
type Specs struct {
	CPU       CPUInfo       `json:"cpu"`
	RAM       RAMInfo       `json:"ram"`
	GPU       *GPUInfo      `json:"gpu"` // nil = no GPU
	Disk      DiskInfo      `json:"disk"`
	OS        string        `json:"os"`
	Python    string        `json:"python"`
	Docker    bool          `json:"docker"`
	LocalOK   bool          `json:"local_models_supported"` // false if arch unsupported
	Models    ModelAdvice   `json:"models"`
}

type CPUInfo struct {
	Arch  string `json:"arch"`  // amd64, arm64
	Cores int    `json:"cores"`
	Model string `json:"model"`
}

type RAMInfo struct {
	TotalGB     float64 `json:"total_gb"`
	AvailableGB float64 `json:"available_gb"`
}

type GPUInfo struct {
	Name   string `json:"name"`
	VRAMGB float64 `json:"vram_gb"`
}

type DiskInfo struct {
	FreeGB float64 `json:"free_gb"`
}

// ModelAdvice recommends which models can run on this system.
type ModelAdvice struct {
	Whisper  WhisperAdvice  `json:"whisper"`
	Kokoro   InstallAdvice  `json:"kokoro"`
	SileroVAD InstallAdvice `json:"silero_vad"`
}

type WhisperAdvice struct {
	Recommended string          `json:"recommended"` // tiny, base, small, medium, large-v3
	Available   []WhisperOption `json:"available"`
}

type WhisperOption struct {
	Model   string  `json:"model"`
	SizeMB  int     `json:"size_mb"`
	RAMGB   float64 `json:"ram_gb"`
	CanRun  bool    `json:"can_run"`
	Note    string  `json:"note,omitempty"`
}

type InstallAdvice struct {
	CanInstall bool   `json:"can_install"`
	Reason     string `json:"reason,omitempty"`
	SizeMB     int    `json:"size_mb"`
}

// Detect gathers system specs and generates model recommendations.
func Detect() Specs {
	s := Specs{
		OS: runtime.GOOS,
		CPU: CPUInfo{
			Arch:  runtime.GOARCH,
			Cores: runtime.NumCPU(),
			Model: cpuModel(),
		},
		RAM:    detectRAM(),
		GPU:    detectGPU(),
		Disk:   detectDisk(),
		Python: detectPython(),
		Docker: detectDocker(),
	}

	// Local models need x86_64 or arm64 Linux/macOS + Python
	s.LocalOK = (s.CPU.Arch == "amd64" || s.CPU.Arch == "arm64") && s.Python != ""

	s.Models = recommendModels(s)
	return s
}

func recommendModels(s Specs) ModelAdvice {
	ram := s.RAM.TotalGB
	hasGPU := s.GPU != nil

	// Whisper model recommendations based on RAM + GPU
	whisperModels := []WhisperOption{
		{Model: "tiny", SizeMB: 75, RAMGB: 1},
		{Model: "base", SizeMB: 150, RAMGB: 1.5},
		{Model: "small", SizeMB: 500, RAMGB: 2.5},
		{Model: "medium", SizeMB: 1500, RAMGB: 5},
		{Model: "large-v3", SizeMB: 3000, RAMGB: 10},
	}

	recommended := "tiny"
	for i := range whisperModels {
		w := &whisperModels[i]
		if ram >= w.RAMGB+2 { // need 2GB headroom
			w.CanRun = true
			recommended = w.Model
		} else {
			w.CanRun = false
			w.Note = fmt.Sprintf("needs %.0fGB RAM (you have %.0fGB)", w.RAMGB+2, ram)
		}
		if !s.LocalOK {
			w.CanRun = false
			w.Note = "unsupported architecture"
		}
		// GPU note
		if w.CanRun && !hasGPU && (w.Model == "medium" || w.Model == "large-v3") {
			w.Note = "will be slow without GPU"
		}
	}

	// Kokoro: needs Python, ~200MB, runs on CPU
	kokoro := InstallAdvice{SizeMB: 200, CanInstall: s.LocalOK}
	if !s.LocalOK {
		kokoro.Reason = "unsupported architecture or Python not found"
	} else if s.RAM.TotalGB < 2 {
		kokoro.CanInstall = false
		kokoro.Reason = "needs at least 2GB RAM"
	}

	// Silero VAD: always available (ONNX, 2MB, runs in browser)
	silero := InstallAdvice{SizeMB: 2, CanInstall: true}

	return ModelAdvice{
		Whisper:   WhisperAdvice{Recommended: recommended, Available: whisperModels},
		Kokoro:    kokoro,
		SileroVAD: silero,
	}
}

func cpuModel() string {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile("/proc/cpuinfo")
		if err != nil { return runtime.GOARCH }
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "Model") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 { return strings.TrimSpace(parts[1]) }
			}
		}
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil { return strings.TrimSpace(string(out)) }
	}
	return runtime.GOARCH
}

func detectRAM() RAMInfo {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil { return RAMInfo{} }
		var total, avail float64
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				total = parseKB(line) / 1024 / 1024
			}
			if strings.HasPrefix(line, "MemAvailable:") {
				avail = parseKB(line) / 1024 / 1024
			}
		}
		return RAMInfo{TotalGB: total, AvailableGB: avail}
	case "darwin":
		out, _ := exec.Command("sysctl", "-n", "hw.memsize").Output()
		bytes, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		return RAMInfo{TotalGB: bytes / 1024 / 1024 / 1024, AvailableGB: bytes / 1024 / 1024 / 1024 * 0.6}
	}
	return RAMInfo{}
}

func parseKB(line string) float64 {
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		v, _ := strconv.ParseFloat(fields[1], 64)
		return v
	}
	return 0
}

func detectGPU() *GPUInfo {
	out, err := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil { return nil }
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) < 2 { return nil }
	vram, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	return &GPUInfo{Name: strings.TrimSpace(parts[0]), VRAMGB: vram / 1024}
}

func detectDisk() DiskInfo {
	switch runtime.GOOS {
	case "linux", "darwin":
		out, err := exec.Command("df", "-BG", "/").Output()
		if err != nil { return DiskInfo{} }
		lines := strings.Split(string(out), "\n")
		if len(lines) < 2 { return DiskInfo{} }
		fields := strings.Fields(lines[1])
		if len(fields) >= 4 {
			free, _ := strconv.ParseFloat(strings.TrimSuffix(fields[3], "G"), 64)
			return DiskInfo{FreeGB: free}
		}
	}
	return DiskInfo{}
}

func detectPython() string {
	for _, cmd := range []string{"python3", "python"} {
		out, err := exec.Command(cmd, "--version").Output()
		if err == nil { return strings.TrimSpace(string(out)) }
	}
	return ""
}

func detectDocker() bool {
	_, err := exec.Command("docker", "info").Output()
	return err == nil
}
