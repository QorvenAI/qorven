// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
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

// ─── Universal OpenAI-compatible voice adapter ─────────────────────────
//
// A single TTS + STT implementation that speaks OpenAI's /v1/audio/*
// contract. Points at the user's configured base URL, so the same
// Go code drives:
//
//   - OpenAI (https://api.openai.com/v1)
//   - Groq (https://api.groq.com/openai/v1) — whisper-large-v3-turbo is
//     the cheapest cloud STT on the market
//   - whisper.cpp server (./main -sv on :8080)
//   - LocalAI (OpenAI-compatible local proxy)
//   - LMNT
//   - LiteLLM proxy (any model behind a single OpenAI-shaped gateway)
//
// The legacy OpenAITTS / WhisperSTT structs in providers.go stay for
// backward compatibility with config.toml bindings. New DB-driven
// rows route through this one instead.
//
// Wire-format spec (OpenAI docs):
//   TTS: POST /audio/speech
//     body: {"model","input","voice","response_format","speed"}
//     returns: raw audio bytes (content-type per response_format)
//
//   STT: POST /audio/transcriptions
//     body: multipart — "file" (audio), "model" (required)
//     returns: {"text":"..."} by default (response_format=json)

// OpenAICompatTTS drives any /v1/audio/speech endpoint.
type OpenAICompatTTS struct {
	apiKey       string
	baseURL      string
	defaultModel string
	defaultVoice string
	client       *http.Client
}

func NewOpenAICompatTTS(apiKey, baseURL, defaultModel, defaultVoice string) *OpenAICompatTTS {
	if baseURL == "" { baseURL = "https://api.openai.com/v1" }
	if defaultModel == "" { defaultModel = "tts-1" }
	if defaultVoice == "" { defaultVoice = "alloy" }
	return &OpenAICompatTTS{
		apiKey:       apiKey,
		baseURL:      strings.TrimRight(baseURL, "/"),
		defaultModel: defaultModel,
		defaultVoice: defaultVoice,
		client:       &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *OpenAICompatTTS) Name() string { return "openai_compat_tts" }

func (p *OpenAICompatTTS) Synthesize(ctx context.Context, text string, opts TTSOptions) (*AudioResult, error) {
	model := firstNonEmpty(opts.Model, p.defaultModel)
	voice := firstNonEmpty(opts.Voice, p.defaultVoice)
	format := firstNonEmpty(opts.Format, "mp3")
	body := map[string]any{
		"model":           model,
		"input":           text,
		"voice":           voice,
		"response_format": format,
	}
	if opts.Speed > 0 {
		body["speed"] = opts.Speed
	}

	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/audio/speech", bytes.NewReader(data))
	if err != nil { return nil, err }
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil { return nil, fmt.Errorf("openai_compat tts: %w", err) }
	defer resp.Body.Close()

	audio, err := io.ReadAll(resp.Body)
	if err != nil { return nil, err }
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai_compat tts: HTTP %d — %s", resp.StatusCode, truncate(string(audio), 200))
	}
	return &AudioResult{
		Audio:     audio,
		Extension: format,
		MimeType:  mimeForFormat(format),
	}, nil
}

// OpenAICompatSTT drives any /v1/audio/transcriptions endpoint.
type OpenAICompatSTT struct {
	apiKey       string
	baseURL      string
	defaultModel string
	client       *http.Client
}

func NewOpenAICompatSTT(apiKey, baseURL, defaultModel string) *OpenAICompatSTT {
	if baseURL == "" { baseURL = "https://api.openai.com/v1" }
	if defaultModel == "" { defaultModel = "whisper-1" }
	return &OpenAICompatSTT{
		apiKey:       apiKey,
		baseURL:      strings.TrimRight(baseURL, "/"),
		defaultModel: defaultModel,
		// STT uploads can be larger; bump timeout a little.
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *OpenAICompatSTT) Name() string { return "openai_compat_stt" }

func (p *OpenAICompatSTT) Transcribe(ctx context.Context, audio []byte, format string) (string, error) {
	if format == "" { format = "webm" }

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "audio."+format)
	if err != nil { return "", err }
	if _, err := fw.Write(audio); err != nil { return "", err }
	_ = w.WriteField("model", p.defaultModel)
	_ = w.WriteField("response_format", "json")
	w.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/audio/transcriptions", &buf)
	if err != nil { return "", err }
	req.Header.Set("Content-Type", w.FormDataContentType())
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil { return "", fmt.Errorf("openai_compat stt: %w", err) }
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil { return "", err }
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai_compat stt: HTTP %d — %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out struct { Text string `json:"text"` }
	if err := json.Unmarshal(body, &out); err != nil {
		// Some backends return raw text for verbose_json or text formats.
		return strings.TrimSpace(string(body)), nil
	}
	return out.Text, nil
}

// ─── tiny helpers ───────────────────────────────────────────────────────

func mimeForFormat(format string) string {
	switch format {
	case "mp3":  return "audio/mpeg"
	case "opus": return "audio/opus"
	case "aac":  return "audio/aac"
	case "flac": return "audio/flac"
	case "wav":  return "audio/wav"
	case "pcm":  return "audio/pcm"
	}
	return "application/octet-stream"
}

func truncate(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "…"
}
