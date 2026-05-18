// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BuildProvider materialises a voice_providers DB row into a concrete
// TTS / STT implementation. The gateway calls this at boot and on
// every POST /v1/voice/providers to get the driver object to
// register with the Manager.
//
// Returns (tts, stt, err). For kind=tts, stt is nil; kind=stt, tts is
// nil; kind=realtime returns (nil, nil, nil) — realtime isn't
// registered on the Manager, it's wired by the gateway's /ws/voice
// handler directly.
//
// Adding a new driver: drop a new case in the switch, ship the
// matching entry in voice_catalog.json, and both the Settings UI and
// the wizard will surface it automatically.
func BuildProvider(row ProviderRow) (TTSProvider, STTProvider, error) {
	settings := parseSettings(row.Settings)

	switch row.Driver {

	// ─── OpenAI-compatible family (drop-in /v1/audio/*) ─────────────────
	// Covers OpenAI itself, Groq, whisper.cpp server, LocalAI, LMNT,
	// LiteLLM proxy — anything that speaks the /v1/audio/speech +
	// /v1/audio/transcriptions contract.
	case "openai_compat":
		base := row.APIBase
		if base == "" { base = "https://api.openai.com/v1" }
		model := settings.String("model")
		voice := settings.String("voice")
		if row.Kind == "tts" {
			return NewOpenAICompatTTS(row.APIKey, base, model, voice), nil, nil
		}
		if row.Kind == "stt" {
			return nil, NewOpenAICompatSTT(row.APIKey, base, model), nil
		}

	// ─── Legacy aliases — same driver, pre-set base URL ────────────────
	case "openai":
		base := "https://api.openai.com/v1"
		if row.APIBase != "" { base = row.APIBase }
		if row.Kind == "tts" {
			return NewOpenAITTS(row.APIKey, base, settings.String("voice")), nil, nil
		}
		if row.Kind == "stt" {
			return nil, NewWhisperSTT(row.APIKey, base), nil
		}
	case "groq":
		base := "https://api.groq.com/openai/v1"
		if row.APIBase != "" { base = row.APIBase }
		if row.Kind == "stt" {
			return nil, NewOpenAICompatSTT(row.APIKey, base, firstNonEmpty(settings.String("model"), "whisper-large-v3-turbo")), nil
		}

	// ─── Native vendor adapters ─────────────────────────────────────────
	case "elevenlabs":
		if row.Kind == "tts" {
			return NewElevenLabsTTS(row.APIKey, settings.String("voice_id")), nil, nil
		}
	case "deepgram":
		if row.Kind == "stt" {
			return nil, NewDeepgramSTT(row.APIKey, firstNonEmpty(settings.String("model"), "nova-3")), nil
		}
	case "cartesia":
		if row.Kind == "tts" {
			return NewCartesiaTTS(row.APIKey, settings.String("voice_id"), firstNonEmpty(settings.String("model"), "sonic-3")), nil, nil
		}
	case "assemblyai":
		if row.Kind == "stt" {
			return nil, NewAssemblyAISTT(row.APIKey, firstNonEmpty(settings.String("model"), "universal")), nil
		}

	// ─── Local / self-hosted ────────────────────────────────────────────
	case "kokoro":
		if row.Kind == "tts" {
			return NewKokoroTTS(row.APIBase, settings.String("voice")), nil, nil
		}
	case "edge_tts":
		if row.Kind == "tts" {
			voice := firstNonEmpty(settings.String("voice"), "en-US-AriaNeural")
			return NewEdgeTTS(voice), nil, nil
		}
	case "faster_whisper":
		if row.Kind == "stt" {
			model := firstNonEmpty(settings.String("model"), "base")
			return nil, NewFasterWhisperSTT(row.APIBase, model), nil
		}
	case "moonshine":
		if row.Kind == "stt" {
			return nil, NewMoonshineSTT(row.APIBase, firstNonEmpty(settings.String("model"), "base")), nil
		}
	case "piper":
		if row.Kind == "tts" {
			modelPath := settings.String("model_path")
			if modelPath == "" {
				return nil, nil, fmt.Errorf("piper driver requires settings.model_path (path to the .onnx voice file)")
			}
			return NewPiperTTS(settings.String("binary"), modelPath), nil, nil
		}

	// ─── Bring-your-own-model ──────────────────────────────────────────
	case "huggingface":
		modelID := settings.String("model_id")
		if modelID == "" {
			return nil, nil, fmt.Errorf("huggingface driver requires settings.model_id")
		}
		if row.Kind == "tts" {
			return NewHuggingFaceTTS(row.APIKey, modelID), nil, nil
		}
		if row.Kind == "stt" {
			return nil, NewHuggingFaceSTT(row.APIKey, modelID), nil
		}
	case "ollama_voice":
		base := firstNonEmpty(row.APIBase, "http://localhost:11434")
		if row.Kind == "tts" {
			return NewOllamaTTS(base, settings.String("model")), nil, nil
		}
		if row.Kind == "stt" {
			return nil, NewOllamaSTT(base, settings.String("model")), nil
		}

	// ─── Realtime — not registered on Manager; caller wires it ─────────
	case "openai_realtime", "gemini_live":
		if row.Kind != "realtime" {
			return nil, nil, fmt.Errorf("driver %q is a realtime driver; row.kind must be 'realtime'", row.Driver)
		}
		return nil, nil, nil
	}

	return nil, nil, fmt.Errorf("voice: unknown driver %q (kind=%s)", row.Driver, row.Kind)
}

// settingsBag is a small helper over json.RawMessage that lets callers
// do settings.String("voice_id") without dragging json decoding into
// every driver. Missing keys or wrong types return the zero value.
type settingsBag map[string]any

func parseSettings(raw json.RawMessage) settingsBag {
	if len(raw) == 0 { return settingsBag{} }
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return settingsBag{}
	}
	return settingsBag(m)
}

func (b settingsBag) String(key string) string {
	if v, ok := b[key]; ok {
		if s, ok := v.(string); ok { return strings.TrimSpace(s) }
	}
	return ""
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs { if x != "" { return x } }
	return ""
}
