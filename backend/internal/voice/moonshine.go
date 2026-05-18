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
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// ─── Moonshine STT (local ONNX model) ──────────────────────────────────
//
// Moonshine is the open-source browser-ready STT that beats Whisper
// large-v3 on streaming WER while running in 26-245M parameters. The
// primary path is ONNX-in-the-browser, but for server-side audio
// uploads we expect a small HTTP wrapper running next to the model:
//
//   https://github.com/usefulsensors/moonshine
//
// The reference server exposes POST /transcribe accepting multipart
// file uploads; our adapter speaks that shape. Users who want
// browser-side STT can ship the ONNX weights from /vendor/moonshine/
// — not wired here, that's a frontend feature.

type MoonshineSTT struct {
	baseURL string
	model   string // "tiny" | "base"
	client  *http.Client
}

// NewMoonshineSTT returns a Moonshine STT adapter. baseURL defaults
// to the local dev port; model defaults to "base" which fits in
// ~245 MB and runs well on a modest laptop CPU.
func NewMoonshineSTT(baseURL, model string) *MoonshineSTT {
	if baseURL == "" { baseURL = "http://localhost:8882" }
	if model == "" { model = "base" }
	return &MoonshineSTT{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *MoonshineSTT) Name() string { return "moonshine" }

func (p *MoonshineSTT) Transcribe(ctx context.Context, audio []byte, format string) (string, error) {
	if format == "" { format = "webm" }

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("file", "audio."+format)
	if _, err := part.Write(audio); err != nil { return "", err }
	_ = w.WriteField("model", p.model)
	w.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/transcribe", &buf)
	if err != nil { return "", err }
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("moonshine stt: %w (is the Moonshine server running at %s?)", err, p.baseURL)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("moonshine stt: HTTP %d — %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out struct{ Text string `json:"text"` }
	if err := json.Unmarshal(body, &out); err != nil {
		return strings.TrimSpace(string(body)), nil
	}
	return strings.TrimSpace(out.Text), nil
}
