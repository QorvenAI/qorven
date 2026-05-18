// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// HotConfig provides thread-safe configuration with file-watching hot-reload.
// Changes take effect on next read without restart.
type HotConfig struct {
	path     string
	current  atomic.Pointer[AppConfig]
	mu       sync.Mutex
	watchers []func(old, new *AppConfig)
	stopCh   chan struct{}
	fsw      *fsnotify.Watcher
}

// AppConfig is the application configuration.
// Add fields as needed — this is the central config struct.
type AppConfig struct {
	// Agent defaults
	DefaultModel     string `json:"default_model"`
	CheapModel       string `json:"cheap_model"`
	MaxIterations    int    `json:"max_iterations"`
	ContextWindow    int    `json:"context_window"`

	// Compaction thresholds
	CompactionBackground float64 `json:"compaction_background"` // default 0.70
	CompactionAggressive float64 `json:"compaction_aggressive"` // default 0.85
	CompactionEmergency  float64 `json:"compaction_emergency"`  // default 0.95

	// Security
	ShellAskMode     string `json:"shell_ask_mode"` // "off", "on-miss", "always"

	// Memory
	MemoryCharLimit  int    `json:"memory_char_limit"`  // default 2200
	UserCharLimit    int    `json:"user_char_limit"`    // default 1375

	// Budget
	DefaultBudgetCents int64 `json:"default_budget_cents"`

	// Heartbeat
	HeartbeatInterval string `json:"heartbeat_interval"` // "5m", "10m", etc.

	// Provider
	FallbackModel    string `json:"fallback_model"`

	// Raw for pass-through
	Raw json.RawMessage `json:"-"`
}

// NewHotConfig loads config from path and starts watching for changes.
func NewHotConfig(path string) *HotConfig {
	hc := &HotConfig{
		path:   path,
		stopCh: make(chan struct{}),
	}

	// Load initial config
	cfg := hc.loadFromDisk()
	hc.current.Store(cfg)

	// Start file watcher (fsnotify if available, fallback to polling)
	if fsw, err := fsnotify.NewWatcher(); err == nil {
		hc.fsw = fsw
		if err := fsw.Add(path); err == nil {
			go hc.watchLoopFsnotify()
			return hc
		}
		fsw.Close()
	}
	go hc.watchLoopPolling()
	return hc
}

// Get returns the current config (lock-free read via atomic).
func (hc *HotConfig) Get() *AppConfig {
	return hc.current.Load()
}

// OnChange registers a callback for config changes.
func (hc *HotConfig) OnChange(fn func(old, new *AppConfig)) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.watchers = append(hc.watchers, fn)
}

// Reload forces a config reload from disk.
func (hc *HotConfig) Reload() {
	old := hc.current.Load()
	new := hc.loadFromDisk()
	hc.current.Store(new)
	hc.notifyWatchers(old, new)
	slog.Info("config.reloaded", "path", hc.path)
}

// Stop halts the file watcher.
func (hc *HotConfig) Stop() {
	close(hc.stopCh)
	if hc.fsw != nil {
		hc.fsw.Close()
	}
}

func (hc *HotConfig) loadFromDisk() *AppConfig {
	cfg := &AppConfig{
		MaxIterations:        20,
		ContextWindow:        128000,
		CompactionBackground: 0.70,
		CompactionAggressive: 0.85,
		CompactionEmergency:  0.95,
		ShellAskMode:         "on-miss",
		MemoryCharLimit:      2200,
		UserCharLimit:        1375,
	}

	data, err := os.ReadFile(hc.path)
	if err != nil {
		slog.Debug("config.load_default", "path", hc.path, "error", err)
		return cfg
	}

	cfg.Raw = data
	if err := json.Unmarshal(data, cfg); err != nil {
		slog.Warn("config.parse_error", "path", hc.path, "error", err)
		return cfg
	}

	return cfg
}

func (hc *HotConfig) watchLoopFsnotify() {
	const debounce = 300 * time.Millisecond
	var timer *time.Timer

	for {
		select {
		case <-hc.stopCh:
			if timer != nil {
				timer.Stop()
			}
			return
		case event, ok := <-hc.fsw.Events:
			if !ok {
				return
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounce, hc.Reload)
		case err, ok := <-hc.fsw.Errors:
			if !ok {
				return
			}
			slog.Error("config.watch_error", "error", err)
		}
	}
}

func (hc *HotConfig) watchLoopPolling() {
	var lastMod time.Time
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-hc.stopCh:
			return
		case <-ticker.C:
			info, err := os.Stat(hc.path)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastMod) {
				lastMod = info.ModTime()
				hc.Reload()
			}
		}
	}
}

func (hc *HotConfig) notifyWatchers(old, new *AppConfig) {
	hc.mu.Lock()
	watchers := make([]func(old, new *AppConfig), len(hc.watchers))
	copy(watchers, hc.watchers)
	hc.mu.Unlock()

	for _, fn := range watchers {
		fn(old, new)
	}
}
