// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import "encoding/json"

// ── Voice providers ───────────────────────────────────────────────────────────

type VoiceProviderInfo struct {
	ID      string // truncated for display
	FullID  string
	Name    string
	Kind    string // tts, stt, realtime
	Driver  string
	Enabled string
}

func (a *apiClient) listVoiceProviders() []VoiceProviderInfo {
	data, err := a.http.Get("/v1/voice/providers")
	if err != nil {
		return nil
	}
	var resp struct {
		Providers []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Kind    string `json:"kind"`
			Driver  string `json:"driver"`
			Enabled bool   `json:"enabled"`
		} `json:"providers"`
	}
	json.Unmarshal(data, &resp)
	var out []VoiceProviderInfo
	for _, p := range resp.Providers {
		enabled := "no"
		if p.Enabled {
			enabled = "yes"
		}
		displayID := p.ID
		if len(displayID) > 8 {
			displayID = displayID[:8]
		}
		out = append(out, VoiceProviderInfo{ID: displayID, FullID: p.ID, Name: p.Name, Kind: p.Kind, Driver: p.Driver, Enabled: enabled})
	}
	return out
}

func (a *apiClient) createVoiceProvider(name, kind, driver, apiBase, apiKey string) error {
	_, err := a.http.Post("/v1/voice/providers", map[string]any{
		"name":     name,
		"kind":     kind,
		"driver":   driver,
		"api_base": apiBase,
		"api_key":  apiKey,
		"enabled":  true,
	})
	return err
}

func (a *apiClient) deleteVoiceProvider(id string) error {
	_, err := a.http.Delete("/v1/voice/providers/" + id)
	return err
}

func (a *apiClient) updateVoiceProvider(id, name, apiBase, apiKey string) error {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if apiBase != "" {
		body["api_base"] = apiBase
	}
	if apiKey != "" {
		body["api_key"] = apiKey
	}
	_, err := a.http.Put("/v1/voice/providers/"+id, body)
	return err
}

// ── Media providers ───────────────────────────────────────────────────────────

type MediaProviderInfo struct {
	ID      string
	Name    string
	Kind    string // image, video
	Driver  string
	Default string
}

func (a *apiClient) listMediaProviders() []MediaProviderInfo {
	data, err := a.http.Get("/v1/media/providers")
	if err != nil {
		return nil
	}
	var resp struct {
		Providers []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Kind      string `json:"kind"`
			Driver    string `json:"driver"`
			IsDefault bool   `json:"is_default"`
		} `json:"providers"`
	}
	json.Unmarshal(data, &resp)
	var out []MediaProviderInfo
	for _, p := range resp.Providers {
		def := "no"
		if p.IsDefault {
			def = "yes"
		}
		id := p.ID
		if len(id) > 8 {
			id = id[:8]
		}
		out = append(out, MediaProviderInfo{ID: id, Name: p.Name, Kind: p.Kind, Driver: p.Driver, Default: def})
	}
	return out
}

// ── Model rankings + router ───────────────────────────────────────────────────

type ModelRankingInfo struct {
	Rank            int
	ID              string
	Name            string
	Organization    string
	IntelligenceIdx float64
	CodingIdx       float64
	SpeedTPS        float64
	InputPricePerM  float64
	OutputPricePerM float64
}

func (a *apiClient) listModelRankings() ([]ModelRankingInfo, bool) {
	data, err := a.http.Get("/v1/routing/model-rankings")
	if err != nil {
		return nil, false
	}
	var resp struct {
		Models     []map[string]any `json:"models"`
		Configured bool             `json:"configured"`
	}
	if json.Unmarshal(data, &resp) != nil || !resp.Configured {
		return nil, resp.Configured
	}
	out := make([]ModelRankingInfo, 0, len(resp.Models))
	for _, m := range resp.Models {
		out = append(out, ModelRankingInfo{
			Rank:            int(floatField(m, "rank")),
			ID:              strField(m, "id"),
			Name:            strField(m, "name"),
			Organization:    strField(m, "organization"),
			IntelligenceIdx: floatField(m, "intelligence_index"),
			CodingIdx:       floatField(m, "coding_index"),
			SpeedTPS:        floatField(m, "speed_tokens_per_sec"),
			InputPricePerM:  floatField(m, "input_price_per_m"),
			OutputPricePerM: floatField(m, "output_price_per_m"),
		})
	}
	return out, true
}

type RouterCategoryInfo struct {
	ID          string
	Name        string
	Description string
	AssignedTo  string
}

func (a *apiClient) listRouterCategories() []RouterCategoryInfo {
	data, err := a.http.Get("/v1/routing/categories")
	if err != nil {
		return nil
	}
	var list []map[string]any
	if json.Unmarshal(data, &list) != nil {
		var resp struct {
			Categories []map[string]any `json:"categories"`
		}
		json.Unmarshal(data, &resp)
		list = resp.Categories
	}

	assignData, _ := a.http.Get("/v1/routing/assignments")
	assignMap := make(map[string]string)
	var assigns []map[string]any
	if json.Unmarshal(assignData, &assigns) == nil {
		for _, a := range assigns {
			assignMap[strField(a, "category_id")] = strField(a, "model_id")
		}
	}

	out := make([]RouterCategoryInfo, 0, len(list))
	for _, c := range list {
		id := strField(c, "id")
		assigned := assignMap[id]
		if assigned == "" {
			assigned = "(auto)"
		}
		desc := strField(c, "description")
		if len(desc) > 35 {
			desc = desc[:35] + "…"
		}
		out = append(out, RouterCategoryInfo{
			ID:          id,
			Name:        strField(c, "name"),
			Description: desc,
			AssignedTo:  assigned,
		})
	}
	return out
}

// ── Integrations ──────────────────────────────────────────────────────────────

type IntegrationInfo struct {
	ID         string
	Name       string
	Configured bool
	KeyHint    string
}

func (a *apiClient) listIntegrations() []IntegrationInfo {
	data, err := a.http.Get("/v1/system/integrations")
	if err != nil {
		return nil
	}
	var resp struct {
		Integrations []map[string]any `json:"integrations"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return nil
	}
	out := make([]IntegrationInfo, 0, len(resp.Integrations))
	for _, i := range resp.Integrations {
		out = append(out, IntegrationInfo{
			ID:         strField(i, "id"),
			Name:       strField(i, "name"),
			Configured: i["configured"] == true,
			KeyHint:    strField(i, "key_hint"),
		})
	}
	return out
}

func (a *apiClient) saveIntegration(id, apiKey string) error {
	_, err := a.http.Post("/v1/system/integrations", map[string]string{"id": id, "api_key": apiKey})
	return err
}
