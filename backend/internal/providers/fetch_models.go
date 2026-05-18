// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// FetchModelsLive fetches models from the provider's API using the given credentials.
func FetchModelsLive(ctx context.Context, providerType, apiBase, apiKey string) ([]ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	switch providerType {
	case "anthropic", "anthropic_native", "Anthropic":
		return fetchAnthropicModels(ctx, apiKey, apiBase)
	case "gemini", "google", "gemini_native":
		return fetchGeminiModels(ctx, apiKey)
	case TypeSageMaker:
		// SageMaker real-time inference has no /v1/models discovery endpoint.
		// Model IDs must be configured manually via the endpoint name.
		return nil, fmt.Errorf("sagemaker: model discovery not supported — configure model IDs via endpoint name")
	default:
		// OpenAI-compatible: GET {base}/models
		return fetchOpenAIModels(ctx, apiBase, apiKey)
	}
}

func fetchOpenAIModels(ctx context.Context, apiBase, apiKey string) ([]ModelInfo, error) {
	base := strings.TrimRight(apiBase, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, ModelInfo{ID: m.ID, Name: m.ID})
	}
	return models, nil
}

func fetchAnthropicModels(ctx context.Context, apiKey, apiBase string) ([]ModelInfo, error) {
	base := strings.TrimRight(apiBase, "/")
	if base == "" {
		base = "https://api.anthropic.com/v1"
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/models", nil)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		name := m.DisplayName
		if name == "" {
			name = m.ID
		}
		models = append(models, ModelInfo{ID: m.ID, Name: name})
	}
	return models, nil
}

func fetchGeminiModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://generativelanguage.googleapis.com/v1beta/models", nil)
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Models []struct {
			Name                 string `json:"name"`
			DisplayName          string `json:"displayName"`
			Description          string `json:"description"`
			InputTokenLimit      int    `json:"inputTokenLimit"`
			OutputTokenLimit     int    `json:"outputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	// Filter: only chat-capable models (gemini, gemma), skip embedding/imagen/veo/lyria/aqa/nano
	skipPrefixes := []string{"imagen", "veo", "lyria", "aqa", "nano", "deep-research"}
	models := make([]ModelInfo, 0)
	for _, m := range result.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		skip := false
		for _, p := range skipPrefixes {
			if strings.HasPrefix(id, p) { skip = true; break }
		}
		// Also skip embedding, tts, audio, image-only, robotics, computer-use models
		if strings.Contains(id, "embedding") || strings.Contains(id, "-tts") ||
			strings.Contains(id, "native-audio") || strings.Contains(id, "robotics") ||
			strings.Contains(id, "computer-use") || strings.Contains(id, "-image") {
			skip = true
		}
		if skip { continue }
		// Only include models that support generateContent (chat)
		supportsChat := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" || method == "streamGenerateContent" {
				supportsChat = true
				break
			}
		}
		if !supportsChat { continue }

		name := m.DisplayName
		if name == "" { name = id }
		models = append(models, ModelInfo{ID: id, Name: name})
	}
	return models, nil
}
