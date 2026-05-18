// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qorvenai/qorven/cmd/client"
	"github.com/qorvenai/qorven/internal/llm"
)

// apiClient is the TUI HTTP client wrapping gateway calls.
type apiClient struct {
	http *client.HTTPClient
}

func newAPI(server, token string) *apiClient {
	return &apiClient{http: client.NewHTTPClient(server, token, false)}
}

// ── Agents ────────────────────────────────────────────────────────────────────

type AgentInfo struct {
	ID    string
	Key   string
	Name  string
	Model string
}

func (a *apiClient) listAgents() []AgentInfo {
	data, err := a.http.Get("/v1/agents")
	if err != nil {
		return nil
	}
	var resp struct {
		Agents []struct {
			ID          string `json:"id"`
			AgentKey    string `json:"agent_key"`
			DisplayName string `json:"display_name"`
			Model       string `json:"model"`
		} `json:"agents"`
	}
	json.Unmarshal(data, &resp)
	var agents []AgentInfo
	for _, a := range resp.Agents {
		if strings.HasPrefix(a.AgentKey, "__") {
			continue
		}
		name := a.DisplayName
		if name == "" {
			name = a.AgentKey
		}
		agents = append(agents, AgentInfo{ID: a.ID, Key: a.AgentKey, Name: name, Model: llm.GetModelName(a.Model)})
	}
	return agents
}

// ── Sessions ──────────────────────────────────────────────────────────────────

type SessionInfo struct {
	ID       string // truncated for display
	FullID   string
	Agent    string
	Channel  string
	Tokens   string
	Updated  string
	MsgCount string
}

func (a *apiClient) listSessions() []SessionInfo {
	data, err := a.http.Get("/v1/sessions")
	if err != nil {
		return nil
	}
	var resp struct {
		Sessions []struct {
			ID           string  `json:"id"`
			AgentID      string  `json:"agent_id"`
			Channel      string  `json:"channel"`
			InputTokens  float64 `json:"input_tokens"`
			OutputTokens float64 `json:"output_tokens"`
			UpdatedAt    string  `json:"updated_at"`
		} `json:"sessions"`
	}
	json.Unmarshal(data, &resp)

	agentNames := make(map[string]string)
	for _, ag := range a.listAgents() {
		agentNames[ag.ID] = ag.Name
	}

	var sessions []SessionInfo
	for _, s := range resp.Sessions {
		updated := s.UpdatedAt
		if len(updated) > 10 {
			updated = updated[:10]
		}
		agentLabel := agentNames[s.AgentID]
		if agentLabel == "" && len(s.AgentID) >= 8 {
			agentLabel = s.AgentID[:8]
		}
		msgCount := ""
		if localMsgs := loadSession(s.ID); len(localMsgs) > 0 {
			msgCount = fmt.Sprintf("%d", len(localMsgs))
		}
		displayID := s.ID
		if len(displayID) > 8 {
			displayID = s.ID[:8]
		}
		sessions = append(sessions, SessionInfo{
			ID:       displayID,
			FullID:   s.ID,
			Agent:    agentLabel,
			Channel:  s.Channel,
			Tokens:   fmt.Sprintf("%d/%d", int(s.InputTokens), int(s.OutputTokens)),
			Updated:  updated,
			MsgCount: msgCount,
		})
	}
	return sessions
}

func (a *apiClient) createSession(agentID string) string {
	data, err := a.http.Post("/v1/sessions", map[string]any{
		"agent_id": agentID, "channel": "tui",
	})
	if err != nil {
		return ""
	}
	var resp struct {
		ID string `json:"id"`
	}
	json.Unmarshal(data, &resp)
	return resp.ID
}

func (a *apiClient) deleteSession(id string) {
	a.http.Delete("/v1/sessions/" + id)
}

// ── Status ────────────────────────────────────────────────────────────────────

func (a *apiClient) getStatus() map[string]string {
	data, err := a.http.Get("/health")
	if err != nil {
		return map[string]string{"status": "offline", "error": err.Error()}
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	result := make(map[string]string)
	for k, v := range resp {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}

// ── Tools ─────────────────────────────────────────────────────────────────────

type ToolInfo struct {
	Name string
	Desc string
}

func (a *apiClient) listTools() []ToolInfo {
	data, err := a.http.Get("/v1/tools/builtin")
	if err != nil {
		return nil
	}
	var list []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if json.Unmarshal(data, &list) != nil {
		var resp struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		}
		json.Unmarshal(data, &resp)
		list = resp.Tools
	}
	var tools []ToolInfo
	for _, t := range list {
		desc := t.Description
		if len(desc) > 50 {
			desc = desc[:50] + "..."
		}
		tools = append(tools, ToolInfo{Name: t.Name, Desc: desc})
	}
	return tools
}

// ── Conversation history ──────────────────────────────────────────────────────

type TUIConversationMessage struct {
	Role          string `json:"role"`
	Content       string `json:"content"`
	DiscussionID  string `json:"discussion_id"`
	SourceChannel string `json:"source_channel"`
}

type TUIDiscussion struct {
	ID        string `json:"id"`
	AILabel   string `json:"ai_label"`
	UserLabel string `json:"user_label"`
}

func (a *apiClient) listAgentMessages(agentID string, limit int) ([]TUIConversationMessage, error) {
	data, err := a.http.Get(fmt.Sprintf("/v1/agents/%s/messages?limit=%d", agentID, limit))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Messages []TUIConversationMessage `json:"messages"`
	}
	json.Unmarshal(data, &resp) //nolint:errcheck
	return resp.Messages, nil
}

func (a *apiClient) listDiscussions(agentID string) ([]TUIDiscussion, error) {
	data, err := a.http.Get(fmt.Sprintf("/v1/agents/%s/discussions", agentID))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Discussions []TUIDiscussion `json:"discussions"`
	}
	json.Unmarshal(data, &resp) //nolint:errcheck
	return resp.Discussions, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// strField is a nil-safe map string extractor for TUI api helpers.
func strField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	default:
		return fmt.Sprintf("%v", v)
	}
}

func floatField(m map[string]any, key string) float64 {
	v, _ := m[key].(float64)
	return v
}

// shortAge converts an ISO timestamp to a human-readable relative age (e.g. "2h", "3d", "1w").
func shortAge(iso string) string {
	if iso == "" {
		return "—"
	}
	formats := []string{
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	var t time.Time
	for _, f := range formats {
		if parsed, err := time.Parse(f, iso); err == nil {
			t = parsed
			break
		}
	}
	if t.IsZero() {
		if len(iso) >= 10 {
			return iso[5:10]
		}
		return iso
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}
}
