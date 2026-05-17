// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import "encoding/json"

// ── Channels ──────────────────────────────────────────────────────────────────

type ChannelInfo struct {
	ID      string // truncated for display
	FullID  string
	Kind    string
	Name    string
	Agent   string
	Status  string
	Enabled string
}

func (a *apiClient) listChannels() []ChannelInfo {
	data, err := a.http.Get("/v1/channels")
	if err != nil {
		return nil
	}
	var resp struct {
		Channels []map[string]any `json:"channels"`
	}
	json.Unmarshal(data, &resp)
	list := resp.Channels
	out := make([]ChannelInfo, 0, len(list))
	for _, c := range list {
		fullID := strField(c, "id")
		displayID := fullID
		if len(displayID) > 8 {
			displayID = displayID[:8]
		}
		out = append(out, ChannelInfo{
			ID:      displayID,
			FullID:  fullID,
			Kind:    strField(c, "channel_type"),
			Name:    strField(c, "name"),
			Agent:   strField(c, "agent_name"),
			Status:  strField(c, "status"),
			Enabled: strField(c, "enabled"),
		})
	}
	return out
}

func (a *apiClient) startChannel(id string) error {
	_, err := a.http.Post("/v1/channels/"+id+"/start", nil)
	return err
}

func (a *apiClient) stopChannel(id string) error {
	_, err := a.http.Post("/v1/channels/"+id+"/stop", nil)
	return err
}

func (a *apiClient) deleteChannel(id string) error {
	_, err := a.http.Delete("/v1/channels/" + id)
	return err
}

func (a *apiClient) updateChannel(id, name, token string) error {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if token != "" {
		body["config"] = map[string]any{"token": token}
	}
	_, err := a.http.Put("/v1/channels/"+id, body)
	return err
}

// ── Rooms (Hubs) ──────────────────────────────────────────────────────────────

type RoomInfo struct {
	ID           string // truncated for display
	FullID       string
	Name         string
	Description  string
	MemberCount  string
	MessageCount string
}

func (a *apiClient) listRooms() []RoomInfo {
	data, err := a.http.Get("/v1/rooms")
	if err != nil {
		return nil
	}
	var resp struct {
		Rooms []map[string]any `json:"rooms"`
	}
	json.Unmarshal(data, &resp)
	list := resp.Rooms
	out := make([]RoomInfo, 0, len(list))
	for _, r := range list {
		fullID := strField(r, "id")
		displayID := fullID
		if len(displayID) > 8 {
			displayID = displayID[:8]
		}
		out = append(out, RoomInfo{
			ID:           displayID,
			FullID:       fullID,
			Name:         strField(r, "display_name"),
			Description:  strField(r, "description"),
			MemberCount:  strField(r, "member_count"),
			MessageCount: strField(r, "message_count"),
		})
	}
	return out
}

func (a *apiClient) deleteRoom(id string) error {
	_, err := a.http.Delete("/v1/rooms/" + id)
	return err
}

// ── Room messages ─────────────────────────────────────────────────────────────

type RoomMessage struct {
	ID        string
	Author    string
	Content   string
	Role      string // "user", "soul", "system"
	CreatedAt string
}

func (a *apiClient) listRoomMessages(roomID string) []RoomMessage {
	data, err := a.http.Get("/v1/rooms/" + roomID + "/messages")
	if err != nil {
		return nil
	}
	var resp struct {
		Messages []map[string]any `json:"messages"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return nil
	}
	out := make([]RoomMessage, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		content := strField(m, "content")
		created := strField(m, "created_at")
		if len(created) > 16 {
			created = created[:16]
		}
		out = append(out, RoomMessage{
			ID:        strField(m, "id"),
			Author:    strField(m, "sender_id"),
			Content:   content,
			Role:      strField(m, "sender_type"),
			CreatedAt: created,
		})
	}
	return out
}

func (a *apiClient) postRoomMessage(roomID, content string) error {
	_, err := a.http.Post("/v1/rooms/"+roomID+"/messages", map[string]any{"content": content})
	return err
}

// ── Skills ────────────────────────────────────────────────────────────────────

type SkillInfo struct {
	ID          string // truncated for display
	FullID      string
	Slug        string
	Name        string
	Description string
	AgentID     string
}

func (a *apiClient) listSkills() []SkillInfo {
	data, err := a.http.Get("/v1/skills")
	if err != nil {
		return nil
	}
	var resp struct {
		Skills []map[string]any `json:"skills"`
	}
	json.Unmarshal(data, &resp)
	list := resp.Skills
	out := make([]SkillInfo, 0, len(list))
	for _, s := range list {
		fullID := strField(s, "id")
		displayID := fullID
		if len(displayID) > 8 {
			displayID = displayID[:8]
		}
		desc := strField(s, "description")
		if len(desc) > 40 {
			desc = desc[:40] + "…"
		}
		out = append(out, SkillInfo{
			ID:          displayID,
			FullID:      fullID,
			Slug:        strField(s, "slug"),
			Name:        strField(s, "name"),
			Description: desc,
			AgentID:     strField(s, "agent_id"),
		})
	}
	return out
}

func (a *apiClient) deleteSkill(id string) error {
	_, err := a.http.Delete("/v1/skills/" + id)
	return err
}

type MarketplaceSkill struct {
	Slug        string
	Name        string
	Description string
}

func (a *apiClient) listMarketplaceSkills() []MarketplaceSkill {
	data, err := a.http.Get("/v1/marketplace/skills")
	if err != nil {
		return nil
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	raw, _ := resp["skills"].([]any)
	out := make([]MarketplaceSkill, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]any)
		desc := strField(m, "description")
		if len(desc) > 50 {
			desc = desc[:50] + "…"
		}
		out = append(out, MarketplaceSkill{
			Slug:        strField(m, "slug"),
			Name:        strField(m, "name"),
			Description: desc,
		})
	}
	return out
}

func (a *apiClient) installSkill(slug string) error {
	_, err := a.http.Post("/v1/marketplace/skills/"+slug+"/install", map[string]any{})
	return err
}

// ── MCP servers ───────────────────────────────────────────────────────────────

type MCPServerInfo struct {
	ID     string
	Name   string
	URL    string
	Status string
}

func (a *apiClient) listMCPServers() []MCPServerInfo {
	data, err := a.http.Get("/v1/mcp/servers")
	if err != nil {
		return nil
	}
	var resp struct {
		Servers []map[string]any `json:"servers"`
	}
	json.Unmarshal(data, &resp)
	list := resp.Servers
	out := make([]MCPServerInfo, 0, len(list))
	for _, s := range list {
		out = append(out, MCPServerInfo{
			ID:     strField(s, "id"),
			Name:   strField(s, "name"),
			URL:    strField(s, "url"),
			Status: strField(s, "status"),
		})
	}
	return out
}

func (a *apiClient) deleteMCPServer(name string) error {
	_, err := a.http.Delete("/v1/mcp/servers/" + name)
	return err
}

// ── Workflows ─────────────────────────────────────────────────────────────────

type WorkflowInfo struct {
	ID          string // truncated for display
	FullID      string
	Name        string
	TriggerType string
	StepCount   int
	Enabled     string
	UpdatedAt   string
}

func (a *apiClient) listWorkflows() []WorkflowInfo {
	data, err := a.http.Get("/v1/workflows")
	if err != nil {
		return nil
	}
	var resp struct {
		Workflows []map[string]any `json:"workflows"`
	}
	json.Unmarshal(data, &resp)
	list := resp.Workflows
	out := make([]WorkflowInfo, 0, len(list))
	for _, w := range list {
		fullID := strField(w, "id")
		displayID := fullID
		if len(displayID) > 8 {
			displayID = displayID[:8]
		}
		updated := strField(w, "updated_at")
		if len(updated) > 10 {
			updated = updated[5:10]
		}
		enabled := "off"
		if e, ok := w["enabled"].(bool); ok && e {
			enabled = "on"
		}
		steps, _ := w["steps"].([]any)
		out = append(out, WorkflowInfo{
			ID:          displayID,
			FullID:      fullID,
			Name:        strField(w, "name"),
			TriggerType: strField(w, "trigger_type"),
			StepCount:   len(steps),
			Enabled:     enabled,
			UpdatedAt:   updated,
		})
	}
	return out
}

func (a *apiClient) runWorkflow(id string) error {
	_, err := a.http.Post("/v1/workflows/"+id+"/run", nil)
	return err
}

func (a *apiClient) deleteWorkflow(id string) error {
	_, err := a.http.Delete("/v1/workflows/" + id)
	return err
}
