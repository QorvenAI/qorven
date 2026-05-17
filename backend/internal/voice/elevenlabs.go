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
	"net/http"
	"time"
)

// ElevenLabs TTS provider — premium natural-sounding voices, 29 languages, voice cloning.
// API: POST https://api.elevenlabs.io/v1/text-to-speech/{voice_id}
// Docs: https://elevenlabs.io/docs/api-reference/text-to-speech/convert

const (
	elevenLabsAPI     = "https://api.elevenlabs.io/v1"
	defaultElevenModel = "eleven_multilingual_v2"
)

type ElevenLabsTTS struct {
	apiKey       string
	defaultVoice string
	model        string
	client       *http.Client
}

type ElevenVoice struct {
	VoiceID  string `json:"voice_id"`
	Name     string `json:"name"`
	Category string `json:"category"` // premade, cloned, generated
	Labels   map[string]string `json:"labels"`
}

func NewElevenLabsTTS(apiKey, defaultVoiceID string) *ElevenLabsTTS {
	if defaultVoiceID == "" { defaultVoiceID = "JBFqnCBsd6RMkjVDRZzb" } // "George" — clear male voice
	return &ElevenLabsTTS{
		apiKey: apiKey, defaultVoice: defaultVoiceID, model: defaultElevenModel,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *ElevenLabsTTS) Name() string { return "elevenlabs" }

func (e *ElevenLabsTTS) Synthesize(ctx context.Context, text string, opts TTSOptions) (*AudioResult, error) {
	if e.apiKey == "" { return nil, fmt.Errorf("elevenlabs: no API key configured") }

	// Clean text for speech
	cleanText := CleanTextForTTS(text)
	if cleanText == "" { return nil, fmt.Errorf("elevenlabs: empty text after cleaning") }

	voiceID := e.defaultVoice
	if opts.Voice != "" { voiceID = opts.Voice }

	// Determine output format
	outputFormat := "mp3_44100_128" // default high quality MP3
	if opts.Format == "ogg" { outputFormat = "mp3_44100_128" } // ElevenLabs doesn't output OGG natively — we'll convert
	if opts.Format == "pcm" { outputFormat = "pcm_44100" }

	// Build request body
	body := map[string]any{
		"text":     cleanText,
		"model_id": e.model,
		"voice_settings": map[string]any{
			"stability":        0.5,
			"similarity_boost": 0.75,
			"style":            0.0,
			"use_speaker_boost": true,
		},
	}
	if opts.Speed > 0 { body["voice_settings"].(map[string]any)["speed"] = opts.Speed }

	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/text-to-speech/%s?output_format=%s", elevenLabsAPI, voiceID, outputFormat)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil { return nil, err }
	req.Header.Set("xi-api-key", e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := e.client.Do(req)
	if err != nil { return nil, fmt.Errorf("elevenlabs: %w", err) }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elevenlabs %d: %s", resp.StatusCode, string(errBody))
	}

	audio, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20)) // 50MB max
	if err != nil { return nil, err }

	return &AudioResult{
		Audio:     audio,
		Extension: "mp3",
		MimeType: "audio/mpeg",
	}, nil
}

// ListVoices returns all available ElevenLabs voices.
func (e *ElevenLabsTTS) ListVoices(ctx context.Context) ([]ElevenVoice, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", elevenLabsAPI+"/voices", nil)
	req.Header.Set("xi-api-key", e.apiKey)
	resp, err := e.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	var result struct {
		Voices []ElevenVoice `json:"voices"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Voices, nil
}

// estimateDuration estimates audio duration from file size and format.
func estimateDuration(bytes int, format string) float64 {
	switch format {
	case "mp3":
		return float64(bytes) / 16000.0 // ~128kbps = 16KB/s
	case "ogg":
		return float64(bytes) / 8000.0 // ~64kbps
	case "wav":
		return float64(bytes) / 88200.0 // 44.1kHz 16-bit mono
	default:
		return 0
	}
}
