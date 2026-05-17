// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package acp

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ACP — Agent Communication Protocol (IBM BeeAI spec)
// REST-based structured messaging between agents in a local/enterprise environment.
// Simpler than A2A — no task state machine, just send/receive messages.

// Envelope is the standard ACP message format.
type Envelope struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`       // agent ID or key
	To        string    `json:"to"`         // agent ID, key, or room ID
	Type      string    `json:"type"`       // "message", "request", "response", "event"
	Content   Content   `json:"content"`
	Metadata  Metadata  `json:"metadata,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type Content struct {
	Text        string         `json:"text,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
	ContentType string         `json:"contentType,omitempty"` // "text/plain", "application/json"
}

type Metadata struct {
	ConversationID string `json:"conversationId,omitempty"`
	ReplyTo        string `json:"replyTo,omitempty"`
	Priority       string `json:"priority,omitempty"` // "low", "normal", "high", "urgent"
	TTL            int    `json:"ttl,omitempty"`      // seconds
}

// AgentInfo describes an ACP-compatible agent.
type AgentInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Protocols   []string `json:"protocols"` // ["acp", "a2a"]
	Endpoint    string   `json:"endpoint"`
}

// Handler provides ACP endpoints for Soul-to-Soul messaging.
type Handler struct {
	sendFunc func(from, to, text string) error // wired to Room/SoulDesk messaging
	listFunc func() []AgentInfo                // list available agents
	inbox    map[string][]Envelope             // agentID → messages (in-memory for now)
}

func NewHandler(sendFunc func(from, to, text string) error, listFunc func() []AgentInfo) *Handler {
	return &Handler{sendFunc: sendFunc, listFunc: listFunc, inbox: make(map[string][]Envelope)}
}

// Routes returns chi routes for ACP endpoints.
func (h *Handler) Routes() func(r chi.Router) {
	return func(r chi.Router) {
		r.Get("/agents", h.handleListAgents)
		r.Post("/messages", h.handleSendMessage)
		r.Get("/agents/{id}/messages", h.handleGetMessages)
	}
}

func (h *Handler) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents := h.listFunc()
	if agents == nil {
		agents = []AgentInfo{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"agents": agents})
}

func (h *Handler) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var env Envelope
	if json.NewDecoder(r.Body).Decode(&env) != nil {
		http.Error(w, `{"error":"invalid envelope"}`, 400)
		return
	}
	if env.From == "" || env.To == "" {
		http.Error(w, `{"error":"from and to required"}`, 400)
		return
	}
	env.ID = uuid.New().String()
	env.Timestamp = time.Now()
	if env.Type == "" { env.Type = "message" }

	// Store in inbox
	h.inbox[env.To] = append(h.inbox[env.To], env)

	// Forward via internal messaging
	if h.sendFunc != nil {
		h.sendFunc(env.From, env.To, env.Content.Text)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": env.ID, "status": "delivered"})
}

func (h *Handler) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	msgs := h.inbox[id]
	if msgs == nil {
		msgs = []Envelope{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"messages": msgs})
}
