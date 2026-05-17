// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package hands

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/subagent"
)

type Hand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Schedule    string `json:"schedule"`
	Status      string `json:"status"` // idle, running, paused
	LastRun     time.Time `json:"last_run"`
	RunCount    int    `json:"run_count"`
}

type Manager struct {
	hands map[string]*Hand
	orch  *subagent.Orchestrator
}

func NewManager(orch *subagent.Orchestrator) *Manager {
	m := &Manager{hands: make(map[string]*Hand), orch: orch}
	m.registerBuiltins()
	return m
}

func (m *Manager) registerBuiltins() {
	builtins := []Hand{
		{Name: "intelligence", Description: "Daily competitive & market intelligence briefing", Schedule: "0 7 * * *"},
		{Name: "researcher", Description: "Deep research on any topic with cited report", Schedule: "manual"},
		{Name: "monitor", Description: "Watch URLs/keywords for changes, alert on detection", Schedule: "*/15 * * * *"},
		{Name: "leadgen", Description: "Discover, qualify, and enrich prospect lists", Schedule: "0 9 * * *"},
		{Name: "report", Description: "Auto-generate weekly business/project reports", Schedule: "0 17 * * 5"},
		{Name: "guardian", Description: "Monitor GitHub repos for issues, CVEs, PR activity", Schedule: "*/30 * * * *"},
		{Name: "knowledge", Description: "Auto-expand knowledge graph from shared content", Schedule: "on-trigger"},
	}
	for _, h := range builtins {
		h.Status = "idle"
		hand := h
		m.hands[h.Name] = &hand
	}
	slog.Info("hands registered", "count", len(m.hands))
}

func (m *Manager) Activate(ctx context.Context, name string) error {
	h, ok := m.hands[name]
	if !ok {
		return fmt.Errorf("hand %q not found", name)
	}
	h.Status = "running"
	slog.Info("hand activated", "name", name)
	return nil
}

func (m *Manager) Pause(name string) error {
	h, ok := m.hands[name]
	if !ok {
		return fmt.Errorf("hand %q not found", name)
	}
	h.Status = "paused"
	return nil
}

func (m *Manager) List() []*Hand {
	out := make([]*Hand, 0, len(m.hands))
	for _, h := range m.hands {
		out = append(out, h)
	}
	return out
}

func (m *Manager) Get(name string) (*Hand, bool) {
	h, ok := m.hands[name]
	return h, ok
}

func (m *Manager) Execute(ctx context.Context, name, input string) (string, error) {
	h, ok := m.hands[name]
	if !ok {
		return "", fmt.Errorf("hand %q not found", name)
	}
	h.Status = "running"
	h.LastRun = time.Now()
	h.RunCount++
	defer func() { h.Status = "idle" }()

	tasks, err := m.orch.Decompose(ctx, fmt.Sprintf("[%s Hand] %s", h.Name, input), 3)
	if err != nil {
		return "", err
	}
	results := m.orch.SpawnParallel(ctx, tasks)

	var output string
	for _, r := range results {
		if r != nil && r.Status == "success" {
			output += r.Content + "\n\n"
		}
	}
	return output, nil
}
