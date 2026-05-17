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
	"log/slog"
	"mime/multipart"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// --- Provider Interface ---

type TTSProvider interface {
	Name() string
	Synthesize(ctx context.Context, text string, opts TTSOptions) (*AudioResult, error)
}

type STTProvider interface {
	Name() string
	Transcribe(ctx context.Context, audio []byte, format string) (string, error)
}

type TTSOptions struct {
	Voice  string `json:"voice"`
	Model  string `json:"model"`
	Format string `json:"format"` // mp3, opus, wav
	Speed  float64 `json:"speed"`
}

type AudioResult struct {
	Audio     []byte `json:"-"`
	Extension string `json:"extension"` // mp3, opus, wav
	MimeType  string `json:"mime_type"`
}

// AutoMode controls when TTS is automatically applied (like Qorven).
type AutoMode string

const (
	AutoOff     AutoMode = "off"
	AutoAlways  AutoMode = "always"
	AutoInbound AutoMode = "inbound" // only if user sent voice
	AutoTagged  AutoMode = "tagged"  // only if reply contains [[tts]]
)

// --- Manager (like Qorven's TTS Manager) ---

type Manager struct {
	ttsProviders map[string]TTSProvider
	sttProviders map[string]STTProvider
	primaryTTS   string
	primarySTT   string
	auto         AutoMode
	maxLength    int
}

func NewManager() *Manager {
	return &Manager{
		ttsProviders: make(map[string]TTSProvider),
		sttProviders: make(map[string]STTProvider),
		auto:         AutoOff,
		maxLength:    4000,
	}
}

func (m *Manager) RegisterTTS(p TTSProvider) {
	m.ttsProviders[p.Name()] = p
	if m.primaryTTS == "" { m.primaryTTS = p.Name() }
	slog.Info("voice: TTS provider registered", "name", p.Name())
}

func (m *Manager) RegisterSTT(p STTProvider) {
	m.sttProviders[p.Name()] = p
	if m.primarySTT == "" { m.primarySTT = p.Name() }
	slog.Info("voice: STT provider registered", "name", p.Name())
}

func (m *Manager) Synthesize(ctx context.Context, text string, opts TTSOptions) (*AudioResult, error) {
	name := m.primaryTTS
	p, ok := m.ttsProviders[name]
	if !ok { return nil, fmt.Errorf("no TTS provider configured") }
	if len(text) > m.maxLength { text = text[:m.maxLength] }
	return p.Synthesize(ctx, text, opts)
}

func (m *Manager) Transcribe(ctx context.Context, audio []byte, format string) (string, error) {
	name := m.primarySTT
	p, ok := m.sttProviders[name]
	if !ok { return "", fmt.Errorf("no STT provider configured") }
	return p.Transcribe(ctx, audio, format)
}

func (m *Manager) SetAuto(mode AutoMode) { m.auto = mode }
func (m *Manager) SetPrimaryTTS(name string) { m.primaryTTS = name }
func (m *Manager) SetPrimarySTT(name string) { m.primarySTT = name }
func (m *Manager) Auto() AutoMode        { return m.auto }
func (m *Manager) HasTTS() bool           { return len(m.ttsProviders) > 0 }
func (m *Manager) HasSTT() bool           { return len(m.sttProviders) > 0 }

func (m *Manager) ListProviders() map[string]any {
	tts := make([]string, 0)
	for n := range m.ttsProviders { tts = append(tts, n) }
	stt := make([]string, 0)
	for n := range m.sttProviders { stt = append(stt, n) }
	return map[string]any{"tts": tts, "stt": stt, "primary_tts": m.primaryTTS, "primary_stt": m.primarySTT, "auto": m.auto}
}

// --- Kokoro TTS (82M, runs on CPU, Apache 2.0) ---
// Calls a local Kokoro FastAPI server: POST /synthesize

type KokoroTTS struct {
	baseURL string
	voice   string
}

func NewKokoroTTS(baseURL, defaultVoice string) *KokoroTTS {
	if baseURL == "" { baseURL = "http://localhost:8880" }
	if defaultVoice == "" { defaultVoice = "af_heart" }
	return &KokoroTTS{baseURL: strings.TrimRight(baseURL, "/"), voice: defaultVoice}
}

func (k *KokoroTTS) Name() string { return "kokoro" }

func (k *KokoroTTS) Synthesize(ctx context.Context, text string, opts TTSOptions) (*AudioResult, error) {
	voice := opts.Voice
	if voice == "" { voice = k.voice }

	body, _ := json.Marshal(map[string]any{
		"text": text, "voice": voice, "speed": opts.Speed,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", k.baseURL+"/synthesize", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil { return nil, fmt.Errorf("kokoro: %w", err) }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kokoro %d: %s", resp.StatusCode, string(b))
	}

	audio, _ := io.ReadAll(resp.Body)
	return &AudioResult{Audio: audio, Extension: "wav", MimeType: "audio/wav"}, nil
}

// --- Edge TTS (free, no API key, Microsoft voices) ---

type EdgeTTS struct {
	voice string
}

func NewEdgeTTS(defaultVoice string) *EdgeTTS {
	if defaultVoice == "" { defaultVoice = "en-US-AriaNeural" }
	return &EdgeTTS{voice: defaultVoice}
}

func (e *EdgeTTS) Name() string { return "edge" }

func (e *EdgeTTS) Synthesize(ctx context.Context, text string, opts TTSOptions) (*AudioResult, error) {
	voice := opts.Voice
	if voice == "" { voice = e.voice }

	// Edge TTS via edge-tts Python CLI (pip install edge-tts)
	cmd := exec.CommandContext(ctx, "edge-tts", "--voice", voice, "--text", text, "--write-media", "/dev/stdout")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("edge-tts: %w (install: pip install edge-tts)", err)
	}

	return &AudioResult{Audio: stdout.Bytes(), Extension: "mp3", MimeType: "audio/mpeg"}, nil
}

// --- OpenAI TTS ---

type OpenAITTS struct {
	apiKey  string
	baseURL string
	model   string
	voice   string
}

func NewOpenAITTS(apiKey, baseURL, defaultVoice string) *OpenAITTS {
	if baseURL == "" { baseURL = "https://api.openai.com/v1" }
	if defaultVoice == "" { defaultVoice = "alloy" }
	return &OpenAITTS{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), model: "tts-1", voice: defaultVoice}
}

func (o *OpenAITTS) Name() string { return "openai" }

func (o *OpenAITTS) Synthesize(ctx context.Context, text string, opts TTSOptions) (*AudioResult, error) {
	voice := opts.Voice
	if voice == "" { voice = o.voice }
	model := opts.Model
	if model == "" { model = o.model }

	body, _ := json.Marshal(map[string]any{
		"model": model, "voice": voice, "input": text,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/audio/speech", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil { return nil, fmt.Errorf("openai tts: %w", err) }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai tts %d: %s", resp.StatusCode, string(b))
	}

	audio, _ := io.ReadAll(resp.Body)
	return &AudioResult{Audio: audio, Extension: "mp3", MimeType: "audio/mpeg"}, nil
}

// --- OpenAI Whisper STT ---

type WhisperSTT struct {
	apiKey  string
	baseURL string
}

func NewWhisperSTT(apiKey, baseURL string) *WhisperSTT {
	if baseURL == "" { baseURL = "https://api.openai.com/v1" }
	return &WhisperSTT{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/")}
}

func (w *WhisperSTT) Name() string { return "whisper" }

func (w *WhisperSTT) Transcribe(ctx context.Context, audio []byte, format string) (string, error) {
	if format == "" { format = "webm" }

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "audio."+format)
	part.Write(audio)
	writer.WriteField("model", "whisper-1")
	writer.Close()

	req, _ := http.NewRequestWithContext(ctx, "POST", w.baseURL+"/audio/transcriptions", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+w.apiKey)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil { return "", fmt.Errorf("whisper: %w", err) }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("whisper %d: %s", resp.StatusCode, string(b))
	}

	var result struct{ Text string `json:"text"` }
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Text, nil
}

// --- Faster-Whisper STT (free, self-hosted, MIT license) ---
// Calls local FastAPI server: POST /transcribe

type FasterWhisperSTT struct {
	baseURL string
	model   string
}

func NewFasterWhisperSTT(baseURL, model string) *FasterWhisperSTT {
	if baseURL == "" { baseURL = "http://localhost:8881" }
	if model == "" { model = "base" }
	return &FasterWhisperSTT{baseURL: strings.TrimRight(baseURL, "/"), model: model}
}

func (f *FasterWhisperSTT) Name() string { return "faster-whisper" }

func (f *FasterWhisperSTT) Transcribe(ctx context.Context, audio []byte, format string) (string, error) {
	if format == "" { format = "webm" }

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "audio."+format)
	part.Write(audio)
	writer.WriteField("model", f.model)
	writer.Close()

	req, _ := http.NewRequestWithContext(ctx, "POST", f.baseURL+"/transcribe", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil { return "", fmt.Errorf("faster-whisper: %w", err) }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("faster-whisper %d: %s", resp.StatusCode, string(b))
	}

	var result struct{ Text string `json:"text"` }
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Text, nil
}

// --- Helpers ---

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
