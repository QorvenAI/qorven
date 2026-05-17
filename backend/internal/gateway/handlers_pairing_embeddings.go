// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/qorvenai/qorven/internal/providers"
)

func (gw *Gateway) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if len(req.Input) == 0 {
		writeJSON(w, 400, map[string]string{"error": "input required"})
		return
	}
	// Forward to configured provider for embeddings
	if gw.providerReg.Default() == nil {
		writeJSON(w, 500, map[string]string{"error": "no provider"})
		return
	}
	cfgs := gw.providerReg.List()
	if len(cfgs) == 0 {
		writeJSON(w, 500, map[string]string{"error": "no provider configured"})
		return
	}
	cfg := cfgs[0]
	// Proxy to the provider's embeddings endpoint
	model := req.Model
	if model == "" {
		model = "text-embedding-3-small"
	}
	body, _ := json.Marshal(map[string]any{"model": model, "input": req.Input})
	proxyReq, _ := http.NewRequestWithContext(r.Context(), "POST",
		cfg.APIBase+"/embeddings", bytes.NewReader(body))
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (gw *Gateway) handleBTW(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID  string `json:"agent_id"`
		Question string `json:"question"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Question == "" {
		writeJSON(w, 400, map[string]string{"error": "question required"})
		return
	}
	provider := gw.providerReg.Default()
	if provider == nil {
		writeJSON(w, 500, map[string]string{"error": "no provider"})
		return
	}
	// Get agent's system prompt for context
	var systemPrompt string
	if req.AgentID != "" && gw.agents != nil {
		if ag, err := gw.agents.Get(r.Context(), req.AgentID); err == nil {
			systemPrompt = ag.SystemPrompt
		}
	}
	if systemPrompt == "" {
		systemPrompt = "You are a helpful assistant. Answer briefly and directly."
	}
	resp, err := provider.Chat(r.Context(), providers.ChatRequest{
		Model: "",
		Messages: []providers.Message{
			{Role: "system", Content: systemPrompt + "\n\nThis is a quick side-question. Answer briefly. Do not use tools."},
			{Role: "user", Content: req.Question},
		},
		Options: map[string]any{"temperature": 0.3, "max_tokens": 300},
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"answer": resp.Content})
}

func (gw *Gateway) handleProviderCatalog(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(providers.ProviderCatalog())
}
