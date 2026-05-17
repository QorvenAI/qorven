// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package a2a

import "time"

// Google A2A Protocol types — https://google.github.io/A2A/
// Full spec compliance for agent discovery and task delegation.

// AgentCard describes an agent's capabilities for discovery.
// Published at /.well-known/agent.json (platform) and /agents/{key}/.well-known/agent.json (per-soul)
type AgentCard struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	URL             string          `json:"url"`
	Provider        *Provider       `json:"provider,omitempty"`
	Version         string          `json:"version"`
	DocumentationURL string         `json:"documentationUrl,omitempty"`
	Capabilities    Capabilities    `json:"capabilities"`
	Skills          []Skill         `json:"skills,omitempty"`
	DefaultInputModes  []string     `json:"defaultInputModes,omitempty"`  // "text", "audio", "image"
	DefaultOutputModes []string     `json:"defaultOutputModes,omitempty"` // "text", "audio", "image"
	Authentication  *Authentication `json:"authentication,omitempty"`
}

type Provider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

type Capabilities struct {
	Streaming          bool `json:"streaming"`
	PushNotifications  bool `json:"pushNotifications"`
	StateTransitions   bool `json:"stateTransitionHistory"`
}

type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
}

type Authentication struct {
	Schemes []AuthScheme `json:"schemes"`
}

type AuthScheme struct {
	Scheme string `json:"scheme"` // "bearer", "apiKey", "oauth2"
}

// Task represents an A2A task lifecycle.
// States: submitted → working → input-required → completed / failed / cancelled
type Task struct {
	ID        string       `json:"id"`
	Status    TaskStatus   `json:"status"`
	Messages  []Message    `json:"messages,omitempty"`
	Artifacts []Artifact   `json:"artifacts,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type TaskStatus string

const (
	TaskSubmitted     TaskStatus = "submitted"
	TaskWorking       TaskStatus = "working"
	TaskInputRequired TaskStatus = "input-required"
	TaskCompleted     TaskStatus = "completed"
	TaskFailed        TaskStatus = "failed"
	TaskCancelled     TaskStatus = "cancelled"
)

type Message struct {
	Role    string `json:"role"` // "user" or "agent"
	Content string `json:"content"`
}

type Artifact struct {
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Data     string `json:"data,omitempty"` // base64 or inline
	URI      string `json:"uri,omitempty"`
}

type SendRequest struct {
	Message string `json:"message"`
}

type TaskResponse struct {
	Task  *Task  `json:"task,omitempty"`
	Error string `json:"error,omitempty"`
}
