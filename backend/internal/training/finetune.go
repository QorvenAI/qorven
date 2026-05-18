// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package training

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FineTuneProvider abstracts fine-tuning API providers.
type FineTuneProvider interface {
	Name() string
	CreateJob(ctx context.Context, req FineTuneRequest) (*FineTuneJob, error)
	GetJob(ctx context.Context, jobID string) (*FineTuneJob, error)
}

type FineTuneRequest struct {
	Model        string `json:"model"`         // base model to fine-tune
	TrainingFile string `json:"training_file"` // JSONL file ID or URL
	Suffix       string `json:"suffix"`        // model name suffix
	Epochs       int    `json:"n_epochs,omitempty"`
}

type FineTuneJob struct {
	ID        string `json:"id"`
	Status    string `json:"status"` // pending, running, succeeded, failed
	Model     string `json:"model"`
	FineTuned string `json:"fine_tuned_model,omitempty"`
	Error     string `json:"error,omitempty"`
}

// --- Together AI ---

type TogetherAI struct {
	apiKey string
}

func NewTogetherAI(apiKey string) *TogetherAI { return &TogetherAI{apiKey: apiKey} }
func (t *TogetherAI) Name() string            { return "together" }

func (t *TogetherAI) CreateJob(ctx context.Context, req FineTuneRequest) (*FineTuneJob, error) {
	body, _ := json.Marshal(map[string]any{
		"model":         req.Model,
		"training_file": req.TrainingFile,
		"suffix":        req.Suffix,
		"n_epochs":      max(req.Epochs, 3),
	})
	return t.doRequest(ctx, "POST", "https://api.together.xyz/v1/fine-tunes", body)
}

func (t *TogetherAI) GetJob(ctx context.Context, jobID string) (*FineTuneJob, error) {
	return t.doRequest(ctx, "GET", "https://api.together.xyz/v1/fine-tunes/"+jobID, nil)
}

func (t *TogetherAI) doRequest(ctx context.Context, method, url string, body []byte) (*FineTuneJob, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, _ := http.NewRequestWithContext(ctx, method, url, bodyReader)
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var job FineTuneJob
	json.NewDecoder(resp.Body).Decode(&job)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("together API %d: %s", resp.StatusCode, job.Error)
	}
	return &job, nil
}

// --- Fireworks AI ---

type FireworksAI struct {
	apiKey    string
	accountID string
}

func NewFireworksAI(apiKey, accountID string) *FireworksAI {
	return &FireworksAI{apiKey: apiKey, accountID: accountID}
}
func (f *FireworksAI) Name() string { return "fireworks" }

func (f *FireworksAI) CreateJob(ctx context.Context, req FineTuneRequest) (*FineTuneJob, error) {
	body, _ := json.Marshal(map[string]any{
		"model":    req.Model,
		"dataset":  req.TrainingFile,
		"suffix":   req.Suffix,
		"epochs":   max(req.Epochs, 3),
	})
	url := fmt.Sprintf("https://api.fireworks.ai/v1/accounts/%s/fineTuningJobs", f.accountID)
	return f.doRequest(ctx, "POST", url, body)
}

func (f *FireworksAI) GetJob(ctx context.Context, jobID string) (*FineTuneJob, error) {
	url := fmt.Sprintf("https://api.fireworks.ai/v1/accounts/%s/fineTuningJobs/%s", f.accountID, jobID)
	return f.doRequest(ctx, "GET", url, nil)
}

func (f *FireworksAI) doRequest(ctx context.Context, method, url string, body []byte) (*FineTuneJob, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, _ := http.NewRequestWithContext(ctx, method, url, bodyReader)
	req.Header.Set("Authorization", "Bearer "+f.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var job FineTuneJob
	json.NewDecoder(resp.Body).Decode(&job)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fireworks API %d: %s", resp.StatusCode, job.Error)
	}
	return &job, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
