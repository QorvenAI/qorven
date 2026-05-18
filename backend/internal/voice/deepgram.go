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

// DeepgramSTT — fast cloud STT with real-time streaming support.
// API: POST https://api.deepgram.com/v1/listen
// Docs: https://developers.deepgram.com/docs/getting-started-with-pre-recorded-audio
type DeepgramSTT struct {
	apiKey string
	model  string
	client *http.Client
}

func NewDeepgramSTT(apiKey, model string) *DeepgramSTT {
	if model == "" { model = "nova-3" }
	return &DeepgramSTT{apiKey: apiKey, model: model, client: &http.Client{Timeout: 30 * time.Second}}
}

func (d *DeepgramSTT) Name() string { return "deepgram" }

func (d *DeepgramSTT) Transcribe(ctx context.Context, audio []byte, format string) (string, error) {
	if d.apiKey == "" { return "", fmt.Errorf("deepgram: no API key") }

	url := fmt.Sprintf("https://api.deepgram.com/v1/listen?model=%s&smart_format=true", d.model)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(audio))
	req.Header.Set("Authorization", "Token "+d.apiKey)

	ct := "audio/webm"
	if format == "wav" { ct = "audio/wav" }
	if format == "mp3" { ct = "audio/mpeg" }
	if format == "ogg" { ct = "audio/ogg" }
	req.Header.Set("Content-Type", ct)

	resp, err := d.client.Do(req)
	if err != nil { return "", fmt.Errorf("deepgram: %w", err) }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("deepgram %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	var result struct {
		Results struct {
			Channels []struct {
				Alternatives []struct {
					Transcript string `json:"transcript"`
				} `json:"alternatives"`
			} `json:"channels"`
		} `json:"results"`
	}
	json.Unmarshal(body, &result)
	if len(result.Results.Channels) > 0 && len(result.Results.Channels[0].Alternatives) > 0 {
		return result.Results.Channels[0].Alternatives[0].Transcript, nil
	}
	return "", nil
}

func min(a, b int) int { if a < b { return a }; return b }
