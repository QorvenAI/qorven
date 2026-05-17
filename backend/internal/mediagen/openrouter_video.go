// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package mediagen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// OpenRouterVideoProvider routes video generation through OpenRouter's unified
// video API — covers Seedance 2.0, Veo 3.1, Hailuo 2.3, Wan 2.7, Kling v3 via
// one API key and one driver.
//
// API: POST /api/v1/videos  →  GET /api/v1/videos/{id}
// Status values: pending | in_progress | completed | failed
type OpenRouterVideoProvider struct {
	apiKey string
	model  string
	client *http.Client
}

func NewOpenRouterVideoProvider(apiKey, model string) *OpenRouterVideoProvider {
	if model == "" {
		model = "bytedance/seedance-2.0"
	}
	return &OpenRouterVideoProvider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *OpenRouterVideoProvider) Name() string { return "openrouter_video" }

func (p *OpenRouterVideoProvider) Generate(ctx context.Context, prompt string, opts VideoOptions) (*VideoResult, error) {
	model := firstNonEmpty(opts.Model, p.model)

	reqBody := map[string]any{
		"model":  model,
		"prompt": prompt,
	}
	if opts.Duration > 0 {
		reqBody["duration"] = opts.Duration
	}
	if opts.AspectRatio != "" {
		reqBody["aspect_ratio"] = opts.AspectRatio
	}
	if opts.Resolution != "" {
		reqBody["resolution"] = opts.Resolution
	}
	if opts.ImageURL != "" {
		reqBody["frame_images"] = []map[string]string{
			{"url": opts.ImageURL, "frame_type": "first"},
		}
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/videos", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter video: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		return nil, fmt.Errorf("openrouter video %d: %s", resp.StatusCode, string(data[:min(len(data), 300)]))
	}

	var task struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		PollingURL string `json:"polling_url"`
	}
	if err := json.Unmarshal(data, &task); err != nil || task.ID == "" {
		return nil, fmt.Errorf("openrouter video: unexpected response: %s", string(data[:min(len(data), 200)]))
	}

	slog.Info("openrouter.video.submitted", "model", model, "task", task.ID)

	pollURL := firstNonEmpty(task.PollingURL, "https://openrouter.ai/api/v1/videos/"+task.ID)
	deadline := time.Now().Add(10 * time.Minute)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return &VideoResult{TaskID: task.ID, PollURL: pollURL}, nil
		case <-time.After(5 * time.Second):
		}

		pollReq, _ := http.NewRequestWithContext(ctx, "GET", pollURL, nil)
		pollReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		pollResp, err := p.client.Do(pollReq)
		if err != nil {
			continue
		}
		pollData, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var result struct {
			Status       string   `json:"status"` // pending | in_progress | completed | failed
			UnsignedURLs []string `json:"unsigned_urls"`
		}
		if json.Unmarshal(pollData, &result) != nil {
			continue
		}
		switch result.Status {
		case "completed":
			if len(result.UnsignedURLs) == 0 {
				return nil, fmt.Errorf("openrouter video: completed but no URLs returned")
			}
			slog.Info("openrouter.video.completed", "model", model, "url", result.UnsignedURLs[0])
			return &VideoResult{URL: result.UnsignedURLs[0], TaskID: task.ID}, nil
		case "failed":
			return nil, fmt.Errorf("openrouter video: task %s failed", task.ID)
		}
	}

	return &VideoResult{TaskID: task.ID, PollURL: pollURL}, nil
}
