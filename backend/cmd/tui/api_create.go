// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import "encoding/json"

// ── Create / mutate operations ────────────────────────────────────────────────

func (a *apiClient) addProviderKey(providerID, apiKey string) error {
	_, err := a.http.Post("/v1/providers/"+providerID+"/keys", map[string]string{"key": apiKey})
	return err
}

func (a *apiClient) uploadDriveFile(filePath string) error {
	_, err := a.http.UploadFile("/v1/drive/upload", filePath, "")
	return err
}

func (a *apiClient) createProvider(providerType, name, apiBase, apiKey string) error {
	body := map[string]any{
		"provider_type": providerType,
		"name":          name,
		"api_base":      apiBase,
	}
	if apiKey != "" {
		body["api_key"] = apiKey
	}
	_, err := a.http.Post("/v1/providers", body)
	return err
}

func (a *apiClient) deleteAgent(id string) error {
	_, err := a.http.Delete("/v1/agents/" + id)
	return err
}

func (a *apiClient) updateAgent(id, name, role, model, systemPrompt string) error {
	body := map[string]any{
		"display_name":  name,
		"role":          role,
		"model":         model,
		"system_prompt": systemPrompt,
	}
	_, err := a.http.Put("/v1/agents/"+id, body)
	return err
}

func (a *apiClient) createAgent(name, role, model, systemPrompt, toolProfile string) error {
	body := map[string]any{
		"display_name":  name,
		"role":          role,
		"model":         model,
		"system_prompt": systemPrompt,
	}
	if toolProfile != "" {
		body["tool_profile"] = toolProfile
	}
	_, err := a.http.Post("/v1/agents", body)
	return err
}

func (a *apiClient) createChannel(channelType, name, agentID, token string) error {
	return a.createChannelFull(channelType, name, agentID, token, "")
}

func (a *apiClient) createChannelFull(channelType, name, agentID, token, contactInfo string) error {
	body := map[string]any{
		"channel_type": channelType,
		"name":         name,
		"agent_id":     agentID,
	}
	cfg := map[string]any{}
	if token != "" {
		cfg["token"] = token
	}
	if contactInfo != "" {
		cfg["contact_info"] = contactInfo
	}
	if len(cfg) > 0 {
		body["config"] = cfg
	}
	_, err := a.http.Post("/v1/channels", body)
	return err
}

func (a *apiClient) createCronJob(agentID, expression, task string) error {
	_, err := a.http.Post("/v1/cron-jobs", map[string]any{
		"agent_id":   agentID,
		"expression": expression,
		"task":       task,
	})
	return err
}

func (a *apiClient) createMCPServer(name, url string) error {
	_, err := a.http.Post("/v1/mcp/servers", map[string]any{
		"name": name,
		"url":  url,
	})
	return err
}

func (a *apiClient) createRoom(name, description string) error {
	_, err := a.http.Post("/v1/rooms", map[string]any{
		"name":         name,
		"display_name": name,
		"description":  description,
	})
	return err
}

func (a *apiClient) createTask(title, agentID, priority string) error {
	priorityMap := map[string]int{"low": 2, "medium": 3, "high": 4}
	p, ok := priorityMap[priority]
	if !ok {
		p = 3
	}
	body := map[string]any{
		"title":    title,
		"priority": p,
	}
	if agentID != "" {
		body["assigned_to"] = agentID
	}
	_, err := a.http.Post("/v1/tasks", body)
	return err
}

func (a *apiClient) createWorkflow(name, description string) (string, error) {
	data, err := a.http.Post("/v1/workflows", map[string]any{
		"name":        name,
		"description": description,
		"enabled":     false,
		"steps":       []any{},
	})
	if err != nil {
		return "", err
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	return strField(resp, "id"), nil
}
