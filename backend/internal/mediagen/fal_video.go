// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
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

// FalVideoProvider routes video generation through Fal.ai's async queue API.
// One API key covers Seedance 2.0, HappyHorse, PixVerse V6, and 100+ other models.
//
// API pattern: POST /fal-queue/submit/{model-id}
//              GET  /fal-queue/{model-id}/requests/{id}/status
//              GET  /fal-queue/{model-id}/requests/{id}/response
type FalVideoProvider struct {
	apiKey  string
	modelID string
	client  *http.Client
}

func NewFalVideoProvider(apiKey, modelID string) *FalVideoProvider {
	if modelID == "" {
		modelID = "bytedance/seedance-2.0/text-to-video"
	}
	return &FalVideoProvider{
		apiKey:  apiKey,
		modelID: modelID,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *FalVideoProvider) Name() string { return "fal_video" }

func (p *FalVideoProvider) Generate(ctx context.Context, prompt string, opts VideoOptions) (*VideoResult, error) {
	modelID := firstNonEmpty(opts.Model, p.modelID)

	reqBody := map[string]any{
		"prompt":     prompt,
		"resolution": "720p",
	}
	if opts.Duration > 0 {
		reqBody["duration"] = opts.Duration
	}
	if opts.AspectRatio != "" {
		reqBody["aspect_ratio"] = opts.AspectRatio
	}
	if opts.ImageURL != "" {
		reqBody["image_url"] = opts.ImageURL
	}
	if opts.EndImageURL != "" {
		reqBody["end_image_url"] = opts.EndImageURL
	}

	body, _ := json.Marshal(reqBody)

	submitURL := "https://queue.fal.run/" + modelID
	req, _ := http.NewRequestWithContext(ctx, "POST", submitURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Key "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fal video submit: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fal video submit %d: %s", resp.StatusCode, string(data[:min(len(data), 300)]))
	}

	var submitted struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(data, &submitted); err != nil || submitted.RequestID == "" {
		return nil, fmt.Errorf("fal video: unexpected submit response: %s", string(data[:min(len(data), 200)]))
	}

	slog.Info("fal.video.submitted", "model", modelID, "request_id", submitted.RequestID)

	statusBase := fmt.Sprintf("https://queue.fal.run/%s/requests/%s", modelID, submitted.RequestID)
	pollURL := statusBase + "/response"
	deadline := time.Now().Add(10 * time.Minute)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return &VideoResult{TaskID: submitted.RequestID, PollURL: pollURL}, nil
		case <-time.After(5 * time.Second):
		}

		statusReq, _ := http.NewRequestWithContext(ctx, "GET", statusBase+"/status", nil)
		statusReq.Header.Set("Authorization", "Key "+p.apiKey)
		statusResp, err := p.client.Do(statusReq)
		if err != nil {
			continue
		}
		statusData, _ := io.ReadAll(statusResp.Body)
		statusResp.Body.Close()

		var status struct {
			Status string `json:"status"` // IN_QUEUE | IN_PROGRESS | COMPLETED | FAILED
		}
		if json.Unmarshal(statusData, &status) != nil {
			continue
		}

		switch status.Status {
		case "COMPLETED":
			resultReq, _ := http.NewRequestWithContext(ctx, "GET", pollURL, nil)
			resultReq.Header.Set("Authorization", "Key "+p.apiKey)
			resultResp, err := p.client.Do(resultReq)
			if err != nil {
				return nil, fmt.Errorf("fal video result fetch: %w", err)
			}
			resultData, _ := io.ReadAll(resultResp.Body)
			resultResp.Body.Close()

			var result struct {
				Video struct {
					URL string `json:"url"`
				} `json:"video"`
			}
			if err := json.Unmarshal(resultData, &result); err != nil || result.Video.URL == "" {
				return nil, fmt.Errorf("fal video: unexpected result: %s", string(resultData[:min(len(resultData), 200)]))
			}
			slog.Info("fal.video.completed", "model", modelID, "url", result.Video.URL)
			return &VideoResult{URL: result.Video.URL, TaskID: submitted.RequestID}, nil

		case "FAILED":
			return nil, fmt.Errorf("fal video: generation failed for request %s", submitted.RequestID)
		}
	}

	return &VideoResult{TaskID: submitted.RequestID, PollURL: pollURL}, nil
}
