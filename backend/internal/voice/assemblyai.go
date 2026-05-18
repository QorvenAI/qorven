// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ─── AssemblyAI STT ────────────────────────────────────────────────────
//
// Native adapter for AssemblyAI's STT. Universal-Streaming gives
// partial + final transcripts over WebSocket; for our one-shot
// Transcribe interface we use their async REST pipeline:
//
//   1. POST /v2/upload  — uploads the audio bytes, returns a URL
//   2. POST /v2/transcript — submits {audio_url}, returns an id
//   3. GET  /v2/transcript/{id} — poll until status ∈ {completed, error}
//
// Async poll adds 1-3s latency vs Groq/Whisper but it's the same
// endpoint their streaming builds on; upgrading to WS later reuses
// the same credentials.
//
// $0.15/hr, speaker diarization + sentiment + topics available via
// settings.extras (passed through to the transcript submit).

const assemblyaiBase = "https://api.assemblyai.com/v2"

type AssemblyAISTT struct {
	apiKey string
	model  string
	client *http.Client
}

// NewAssemblyAISTT returns a REST-pipeline STT adapter. `model` is
// passed as speech_model in the transcript submit (e.g.
// "universal" or "nano"); empty picks AssemblyAI's default.
func NewAssemblyAISTT(apiKey, model string) *AssemblyAISTT {
	return &AssemblyAISTT{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *AssemblyAISTT) Name() string { return "assemblyai" }

func (p *AssemblyAISTT) Transcribe(ctx context.Context, audio []byte, _ string) (string, error) {
	// 1. Upload audio. The /v2/upload endpoint accepts the raw bytes
	// with no multipart wrapping; returns {upload_url}.
	uploadURL, err := p.upload(ctx, audio)
	if err != nil { return "", err }

	// 2. Submit transcript job.
	id, err := p.submit(ctx, uploadURL)
	if err != nil { return "", err }

	// 3. Poll until terminal. AssemblyAI transcripts for short clips
	// (< 30s) typically complete in 2-3 seconds. Cap total wait at
	// 90s so a broken job doesn't hang the handler.
	return p.poll(ctx, id, 90*time.Second)
}

func (p *AssemblyAISTT) upload(ctx context.Context, audio []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", assemblyaiBase+"/upload", bytes.NewReader(audio))
	if err != nil { return "", err }
	req.Header.Set("Authorization", p.apiKey)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := p.client.Do(req)
	if err != nil { return "", fmt.Errorf("assemblyai upload: %w", err) }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("assemblyai upload: HTTP %d — %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out struct{ UploadURL string `json:"upload_url"` }
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("assemblyai upload: decode %w", err)
	}
	return out.UploadURL, nil
}

func (p *AssemblyAISTT) submit(ctx context.Context, uploadURL string) (string, error) {
	payload := map[string]any{"audio_url": uploadURL}
	if p.model != "" {
		payload["speech_model"] = p.model
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", assemblyaiBase+"/transcript", bytes.NewReader(body))
	if err != nil { return "", err }
	req.Header.Set("Authorization", p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil { return "", fmt.Errorf("assemblyai submit: %w", err) }
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("assemblyai submit: HTTP %d — %s", resp.StatusCode, truncate(string(rb), 200))
	}
	var out struct{ ID string `json:"id"` }
	if err := json.Unmarshal(rb, &out); err != nil {
		return "", fmt.Errorf("assemblyai submit: decode %w", err)
	}
	return out.ID, nil
}

func (p *AssemblyAISTT) poll(ctx context.Context, id string, maxWait time.Duration) (string, error) {
	deadline := time.Now().Add(maxWait)
	backoff := 750 * time.Millisecond
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backoff):
		}
		// Exponential-ish backoff capped at 3s so long clips don't
		// hammer the poll endpoint.
		if backoff < 3*time.Second {
			backoff = backoff + (backoff / 2)
		}

		req, err := http.NewRequestWithContext(ctx, "GET", assemblyaiBase+"/transcript/"+id, nil)
		if err != nil { return "", err }
		req.Header.Set("Authorization", p.apiKey)
		resp, err := p.client.Do(req)
		if err != nil { return "", fmt.Errorf("assemblyai poll: %w", err) }
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			return "", fmt.Errorf("assemblyai poll: HTTP %d — %s", resp.StatusCode, truncate(string(body), 200))
		}
		var out struct {
			Status string `json:"status"`
			Text   string `json:"text"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return "", fmt.Errorf("assemblyai poll: decode %w", err)
		}
		switch out.Status {
		case "completed":
			return out.Text, nil
		case "error":
			return "", fmt.Errorf("assemblyai: %s", out.Error)
		}
	}
	return "", fmt.Errorf("assemblyai poll: timed out after %s", maxWait)
}
