// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// EmbeddingClient generates vector embeddings via an OpenAI-compatible API.
type EmbeddingClient struct {
	baseURL    string
	model      string
	apiKey     string
	dimensions int // 0 = model default
	client     *http.Client
}

func NewEmbeddingClient(baseURL, model string) *EmbeddingClient {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &EmbeddingClient{baseURL: baseURL, model: model, client: &http.Client{}}
}

// WithAPIKey sets the API key for authenticated requests.
func (e *EmbeddingClient) WithAPIKey(key string) *EmbeddingClient {
	e.apiKey = key
	return e
}

// WithDimensions sets the output dimension truncation.
func (e *EmbeddingClient) WithDimensions(dims int) *EmbeddingClient {
	e.dimensions = dims
	return e
}

// Name returns the provider name for logging.
func (e *EmbeddingClient) Name() string { return e.baseURL }

// Model returns the embedding model name.
func (e *EmbeddingClient) Model() string { return e.model }

// Embed generates a vector embedding for the given text.
func (e *EmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	return results[0], nil
}

// EmbedBatch generates embeddings for multiple texts in one API call.
func (e *EmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"model": e.model,
		"input": texts,
	})
	if e.dimensions > 0 {
		var m map[string]any
		json.Unmarshal(body, &m)
		m["dimensions"] = e.dimensions
		body, _ = json.Marshal(m)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}

	embeddings := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}
	return embeddings, nil
}
