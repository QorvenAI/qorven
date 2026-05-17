// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ─── Ollama voice adapters ─────────────────────────────────────────────
//
// Ollama is gaining first-class audio model support. Two patterns
// shipped across recent releases:
//
//   TTS: POST /api/generate with an audio-producing model (e.g.
//        suno/bark, xtts-v2). Response is streamed JSON lines; the
//        final line carries {done: true, audio: base64}. Some
//        community builds embed the audio in the response directly
//        as audio/wav.
//   STT: POST /api/transcribe — streaming audio input, JSON output
//        {"text": "..."} (available in Ollama >= 0.5 when the model
//        is whisper.cpp-compatible).
//
// Both endpoints are polymorphic: the same URL accepts different
// model families. Our adapter just points at the user's Ollama host
// (default http://localhost:11434) and sends the model name.
//
// Not-supported behaviour: when the local Ollama is older and
// doesn't have these endpoints, we return a structured error
// rather than a timeout — the Settings page surfaces this to the
// user as "Upgrade Ollama to version X to use voice models."

type OllamaTTS struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaTTS(baseURL, model string) *OllamaTTS {
	if baseURL == "" { baseURL = "http://localhost:11434" }
	return &OllamaTTS{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OllamaTTS) Name() string { return "ollama_tts" }

func (p *OllamaTTS) Synthesize(ctx context.Context, text string, _ TTSOptions) (*AudioResult, error) {
	body, _ := json.Marshal(map[string]any{
		"model":  p.model,
		"prompt": text,
		"stream": false,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil { return nil, err }
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		// Connection refused = Ollama not running. Surface a
		// recognisable error so the Settings UI can guide the user.
		return nil, fmt.Errorf("ollama tts: %w (is Ollama running at %s?)", err, p.baseURL)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 404 {
		return nil, errors.New("ollama tts: /api/generate returned 404 — update Ollama to a version that supports audio models")
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama tts: HTTP %d — %s", resp.StatusCode, truncate(string(raw), 200))
	}

	// Response may be:
	//   (a) raw audio bytes (Ollama's newer audio endpoint)
	//   (b) JSON {"response":"base64...","done":true}
	if looksLikeAudio(raw) {
		return &AudioResult{Audio: raw, Extension: "wav", MimeType: "audio/wav"}, nil
	}
	var out struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ollama tts: cannot parse response — %w", err)
	}
	audio, err := base64.StdEncoding.DecodeString(out.Response)
	if err != nil { return nil, fmt.Errorf("ollama tts: b64 decode: %w", err) }
	return &AudioResult{Audio: audio, Extension: "wav", MimeType: "audio/wav"}, nil
}

type OllamaSTT struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaSTT(baseURL, model string) *OllamaSTT {
	if baseURL == "" { baseURL = "http://localhost:11434" }
	return &OllamaSTT{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OllamaSTT) Name() string { return "ollama_stt" }

func (p *OllamaSTT) Transcribe(ctx context.Context, audio []byte, format string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":  p.model,
		"audio":  base64.StdEncoding.EncodeToString(audio),
		"format": format,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/transcribe", bytes.NewReader(body))
	if err != nil { return "", err }
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama stt: %w (is Ollama running at %s?)", err, p.baseURL)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 404 {
		return "", errors.New("ollama stt: /api/transcribe returned 404 — update Ollama to a version that supports STT")
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama stt: HTTP %d — %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var out struct{ Text string `json:"text"` }
	if err := json.Unmarshal(raw, &out); err == nil {
		return strings.TrimSpace(out.Text), nil
	}
	return strings.TrimSpace(string(raw)), nil
}

// looksLikeAudio returns true for WAV / OGG / MP3 magic bytes.
func looksLikeAudio(b []byte) bool {
	if len(b) < 4 { return false }
	switch string(b[:4]) {
	case "RIFF", "OggS":
		return true
	}
	return len(b) > 3 && string(b[:3]) == "ID3"
}
