// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/agent"
)

// RunFunc executes an agent and returns the response text.
type RunFunc func(ctx context.Context, agentID, message string) (string, error)

// Server handles A2A protocol endpoints.
type Server struct {
	agentStore *agent.Store
	baseURL    string
	runAgent   RunFunc
	mu         sync.RWMutex
	tasks      map[string]*Task
}

func NewServer(agentStore *agent.Store, baseURL string, runAgent RunFunc) *Server {
	return &Server{
		agentStore: agentStore,
		baseURL:    baseURL,
		runAgent:   runAgent,
		tasks:      make(map[string]*Task),
	}
}

// Routes returns chi routes for A2A endpoints.
func (s *Server) Routes() func(r chi.Router) {
	return func(r chi.Router) {
		r.Get("/.well-known/agent.json", s.handlePlatformCard)
		r.Get("/agents/{key}/.well-known/agent.json", s.handleAgentCard)
		r.Post("/agents/{key}/tasks", s.handleCreateTask)
		r.Get("/tasks/{id}", s.handleGetTask)
		r.Post("/tasks/{id}/send", s.handleSendMessage)
	}
}

func (s *Server) handlePlatformCard(w http.ResponseWriter, r *http.Request) {
	// List all active souls as skills
	agents, _ := s.agentStore.List(r.Context(), "")
	skills := []Skill{}
	for _, ag := range agents {
		skills = append(skills, Skill{
			ID: ag.AgentKey, Name: ag.DisplayName,
			Description: deref(ag.Role) + " — " + deref(ag.Title),
			Tags: []string{deref(ag.Role), ag.ToolProfile},
		})
	}

	card := AgentCard{
		Name:        "Qorven",
		Description: "Open-source multi-agent AI platform with specialist Souls. Each Soul is an autonomous agent with unique skills, tools, and knowledge.",
		URL:         s.baseURL,
		Provider:    &Provider{Organization: "Qorven", URL: "https://qorven.ai"},
		Version:     "2.0.0",
		DocumentationURL: "https://docs.qorven.ai",
		Capabilities: Capabilities{Streaming: true, PushNotifications: false, StateTransitions: true},
		Skills:          skills,
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Authentication: &Authentication{Schemes: []AuthScheme{{Scheme: "bearer"}}},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

func (s *Server) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	ag, err := s.agentStore.GetByKey(r.Context(), key)
	if err != nil {
		http.Error(w, `{"error":"agent not found"}`, 404)
		return
	}

	desc := deref(ag.Role) + " — " + deref(ag.Title)
	if ag.SystemPrompt != "" {
		end := len(ag.SystemPrompt)
		if end > 300 { end = 300 }
		desc = ag.SystemPrompt[:end]
	}

	// Build skills from tool profile
	skills := []Skill{}
	switch ag.ToolProfile {
	case "coding":
		skills = append(skills, Skill{ID: "code", Name: "Code Generation", Description: "Write, debug, and review code", Tags: []string{"coding", "debugging"}})
		skills = append(skills, Skill{ID: "exec", Name: "Code Execution", Description: "Execute code in sandboxed environment", Tags: []string{"sandbox", "python", "bash"}})
	case "research":
		skills = append(skills, Skill{ID: "search", Name: "Web Research", Description: "Search the web and synthesize findings", Tags: []string{"search", "analysis"}})
		skills = append(skills, Skill{ID: "deep_research", Name: "Deep Research", Description: "Multi-step agentic research with citations", Tags: []string{"research", "citations"}})
	case "full":
		skills = append(skills,
			Skill{ID: "chat", Name: "Conversation", Description: "General conversation and task delegation", Tags: []string{"chat"}},
			Skill{ID: "tools", Name: "Tool Use", Description: "Execute tools and interact with external systems", Tags: []string{"tools", "mcp"}},
		)
	}
	skills = append(skills, Skill{ID: "chat", Name: "Chat", Description: "Conversational interaction", Tags: []string{"chat", "text"}})

	card := AgentCard{
		Name:        ag.DisplayName,
		Description: desc,
		URL:         s.baseURL + "/a2a/agents/" + key,
		Provider:    &Provider{Organization: "Qorven", URL: "https://qorven.ai"},
		Version:     "2.0.0",
		Capabilities: Capabilities{Streaming: true, PushNotifications: false, StateTransitions: true},
		Skills:          skills,
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Authentication: &Authentication{Schemes: []AuthScheme{{Scheme: "bearer"}}},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var req SendRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Message == "" {
		http.Error(w, `{"error":"message required"}`, 400)
		return
	}

	ag, err := s.agentStore.GetByKey(r.Context(), key)
	if err != nil {
		http.Error(w, `{"error":"agent not found"}`, 404)
		return
	}

	task := &Task{
		ID:        uuid.New().String(),
		Status:    TaskWorking,
		Messages:  []Message{{Role: "user", Content: req.Message}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.mu.Lock()
	s.tasks[task.ID] = task
	s.mu.Unlock()

	// Run agent synchronously
	resp, err := s.runAgent(r.Context(), ag.ID, req.Message)
	s.mu.Lock()
	if err != nil {
		task.Status = TaskFailed
	} else {
		task.Status = TaskCompleted
		task.Messages = append(task.Messages, Message{Role: "agent", Content: resp})
	}
	task.UpdatedAt = time.Now()
	s.mu.Unlock()

	json.NewEncoder(w).Encode(TaskResponse{Task: task})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.mu.RLock()
	task, ok := s.tasks[id]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, `{"error":"task not found"}`, 404)
		return
	}
	json.NewEncoder(w).Encode(TaskResponse{Task: task})
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.mu.RLock()
	task, ok := s.tasks[id]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, `{"error":"task not found"}`, 404)
		return
	}

	var req SendRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Message == "" {
		http.Error(w, `{"error":"message required"}`, 400)
		return
	}

	// Find agent from first message context
	// For now, use the task's existing agent
	s.mu.Lock()
	task.Messages = append(task.Messages, Message{Role: "user", Content: req.Message})
	task.Status = TaskWorking
	s.mu.Unlock()

	json.NewEncoder(w).Encode(TaskResponse{Task: task})
}

func deref(s *string) string { if s != nil { return *s }; return "" }
