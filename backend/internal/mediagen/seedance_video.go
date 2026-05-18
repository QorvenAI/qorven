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

// SeedanceVideoProvider calls the official ByteDance/BytePlus Seedance API
// directly (VolcEngine Visual API). This is the native endpoint — no third-party
// proxy. Requires a BytePlus account and Visual API access.
//
// Base URL:  https://visual.byteplus.com
// Auth:      Authorization: AccessKey {key}
// Endpoint:  POST /cv/v1/submit_task  →  POST /cv/v1/get_task_result
// Model IDs: seedance-2.0, seedance-1.5, seedance-1.5-lite
//
// Note: BytePlus/VolcEngine rate-limits per-account. API keys are issued
// via https://console.byteplus.com/visual
type SeedanceVideoProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewSeedanceVideoProvider(apiKey, model string) *SeedanceVideoProvider {
	if model == "" {
		model = "seedance-2.0"
	}
	return &SeedanceVideoProvider{
		apiKey:  apiKey,
		baseURL: "https://visual.byteplus.com",
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *SeedanceVideoProvider) Name() string { return "seedance" }

func (p *SeedanceVideoProvider) Generate(ctx context.Context, prompt string, opts VideoOptions) (*VideoResult, error) {
	model := firstNonEmpty(opts.Model, p.model)
	dur := opts.Duration
	if dur <= 0 {
		dur = 5
	}
	ratio := firstNonEmpty(opts.AspectRatio, "16:9")

	req_data := map[string]any{
		"prompt":       prompt,
		"model_id":     model,
		"duration":     dur,
		"aspect_ratio": ratio,
		"resolution":   firstNonEmpty(opts.Resolution, "720p"),
	}
	if opts.ImageURL != "" {
		req_data["image_url"] = opts.ImageURL
	}
	if opts.EndImageURL != "" {
		req_data["end_image_url"] = opts.EndImageURL
	}

	submitBody := map[string]any{
		"req_key":  "seedance_t2v",
		"req_data": req_data,
	}
	if opts.ImageURL != "" {
		submitBody["req_key"] = "seedance_i2v"
	}

	body, _ := json.Marshal(submitBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/cv/v1/submit_task", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "AccessKey "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("seedance submit: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("seedance submit %d: %s", resp.StatusCode, string(data[:min(len(data), 300)]))
	}

	var submitted struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &submitted); err != nil || submitted.Data.TaskID == "" {
		return nil, fmt.Errorf("seedance: unexpected submit response: %s", string(data[:min(len(data), 200)]))
	}
	if submitted.Code != 0 {
		return nil, fmt.Errorf("seedance: submit error %d: %s", submitted.Code, submitted.Message)
	}

	taskID := submitted.Data.TaskID
	slog.Info("seedance.video.submitted", "model", model, "task_id", taskID)

	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return &VideoResult{TaskID: taskID}, nil
		case <-time.After(5 * time.Second):
		}

		pollBody, _ := json.Marshal(map[string]string{"task_id": taskID})
		pollReq, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/cv/v1/get_task_result", bytes.NewReader(pollBody))
		pollReq.Header.Set("Content-Type", "application/json")
		pollReq.Header.Set("Authorization", "AccessKey "+p.apiKey)
		pollResp, err := p.client.Do(pollReq)
		if err != nil {
			continue
		}
		pollData, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var result struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
			Data struct {
				Status string `json:"status"` // submitted | processing | succeed | failed
				Videos []struct {
					URL string `json:"url"`
				} `json:"video_list"`
			} `json:"data"`
		}
		if json.Unmarshal(pollData, &result) != nil {
			continue
		}
		switch result.Data.Status {
		case "succeed":
			if len(result.Data.Videos) == 0 {
				return nil, fmt.Errorf("seedance: succeed but no video URLs in response")
			}
			url := result.Data.Videos[0].URL
			slog.Info("seedance.video.completed", "model", model, "url", url)
			return &VideoResult{URL: url, TaskID: taskID, Duration: dur}, nil
		case "failed":
			return nil, fmt.Errorf("seedance: generation failed for task %s: %s", taskID, result.Msg)
		}
	}

	return &VideoResult{TaskID: taskID}, nil
}
