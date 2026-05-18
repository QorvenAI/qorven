// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scenario

import (
	"encoding/json"
	"net/http"

	"github.com/qorvenai/qorven/internal/providers"
)

type Handlers struct {
	store    *Store
	engine   *Engine
	provider providers.Provider
}

func NewHandlers(store *Store, provider providers.Provider) *Handlers {
	return &Handlers{store: store, engine: NewEngine(provider), provider: provider}
}

func (h *Handlers) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Seed       string `json:"seed"`
		AgentCount int    `json:"agent_count"`
		Rounds     int    `json:"rounds"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Seed == "" { http.Error(w, `{"error":"seed required"}`, 400); return }
	if req.AgentCount < 2 { req.AgentCount = 5 }
	if req.Rounds < 1 { req.Rounds = 5 }
	if req.Name == "" { req.Name = "Scenario" }

	p, err := h.store.Create(r.Context(), "default", req.Name, req.Seed, req.AgentCount, req.Rounds)
	if err != nil { http.Error(w, `{"error":"`+err.Error()+`"}`, 500); return }

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.Get(r.Context(), id)
	if err != nil { http.Error(w, `{"error":"not found"}`, 404); return }
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	projects, _ := h.store.List(r.Context(), "default")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"scenarios": projects})
}

func (h *Handlers) HandleRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.Get(r.Context(), id)
	if err != nil { http.Error(w, `{"error":"not found"}`, 404); return }

	// Run async
	go func() {
		ctx := r.Context()
		h.store.UpdateStatus(ctx, id, StatusRunning)

		// 1. Generate personas
		agents, err := h.engine.GeneratePersonas(ctx, p.Seed, p.AgentCount)
		if err != nil { h.store.UpdateStatus(ctx, id, StatusFailed); return }

		// 2. Run simulation
		rounds, err := h.engine.RunSimulation(ctx, p.Seed, agents, p.Rounds, nil)
		if err != nil { h.store.UpdateStatus(ctx, id, StatusFailed); return }
		h.store.SaveRounds(ctx, id, rounds)

		// 3. Generate report
		report, err := h.engine.GenerateReport(ctx, p.Seed, agents, rounds)
		if err != nil { h.store.UpdateStatus(ctx, id, StatusFailed); return }
		h.store.SaveReport(ctx, id, report)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started", "id": id})
}

func (h *Handlers) HandleInject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct{ Event string `json:"event"` }
	json.NewDecoder(r.Body).Decode(&req)
	if req.Event == "" { http.Error(w, `{"error":"event required"}`, 400); return }

	p, err := h.store.Get(r.Context(), id)
	if err != nil { http.Error(w, `{"error":"not found"}`, 404); return }

	rounds, _ := h.store.GetRounds(r.Context(), id)
	agents := p.Agents
	if len(agents) == 0 {
		agents, _ = h.engine.GeneratePersonas(r.Context(), p.Seed, p.AgentCount)
	}

	newRounds, err := h.engine.InjectEvent(r.Context(), p.Seed, agents, rounds, req.Event)
	if err != nil { http.Error(w, `{"error":"`+err.Error()+`"}`, 500); return }
	h.store.SaveRounds(r.Context(), id, newRounds)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"rounds": newRounds})
}
