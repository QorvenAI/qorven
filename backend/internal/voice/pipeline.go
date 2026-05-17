// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package voice

import (
	"context"
	"fmt"
	"log/slog"
)

// VoicePipeline handles the complete voice-to-voice flow:
// Audio in → STT transcription → Agent processing → TTS synthesis → Audio out
//
// This is the core component that channels use for voice messages.

type VoicePipeline struct {
	manager   *Manager
	agentChat func(ctx context.Context, agentID, sessionID, message string) (string, error)
}

func NewVoicePipeline(manager *Manager, agentChat func(ctx context.Context, agentID, sessionID, message string) (string, error)) *VoicePipeline {
	return &VoicePipeline{manager: manager, agentChat: agentChat}
}

// TranscribeAudio takes raw audio bytes and returns the transcript.
// This is what channels call when they receive a voice message.
func (p *VoicePipeline) TranscribeAudio(ctx context.Context, audio []byte, format string) (string, error) {
	if !p.manager.HasSTT() {
		return "", fmt.Errorf("no STT provider configured — set up Whisper or faster-whisper")
	}
	if format == "" { format = DetectAudioFormat(audio) }
	transcript, err := p.manager.Transcribe(ctx, audio, format)
	if err != nil {
		slog.Warn("voice.pipeline.stt_failed", "format", format, "bytes", len(audio), "error", err)
		return "", err
	}
	slog.Info("voice.pipeline.transcribed", "format", format, "bytes", len(audio), "transcript_len", len(transcript))
	return transcript, nil
}

// SynthesizeSpeech takes text and returns audio bytes.
// This is what channels call to send a voice reply.
func (p *VoicePipeline) SynthesizeSpeech(ctx context.Context, text, channelType, voice string) (*AudioResult, error) {
	if !p.manager.HasTTS() {
		return nil, fmt.Errorf("no TTS provider configured — set up Kokoro, Edge, OpenAI, or ElevenLabs")
	}
	cleanText := CleanTextForTTS(text)
	if cleanText == "" { return nil, fmt.Errorf("empty text after cleaning") }

	format := PlatformAudioFormat(channelType)
	result, err := p.manager.Synthesize(ctx, cleanText, TTSOptions{Voice: voice, Format: format})
	if err != nil {
		slog.Warn("voice.pipeline.tts_failed", "channel", channelType, "error", err)
		return nil, err
	}
	slog.Info("voice.pipeline.synthesized", "channel", channelType, "format", format, "bytes", len(result.Audio))
	return result, nil
}

// ProcessVoiceMessage handles the full voice-to-voice flow:
// 1. Transcribe incoming audio
// 2. Run through agent
// 3. Synthesize response as audio
// Returns: (audioResult, textResponse, transcript, error)
func (p *VoicePipeline) ProcessVoiceMessage(ctx context.Context, audio []byte, format, agentID, sessionID, channelType, voice string) (*AudioResult, string, string, error) {
	// Step 1: Transcribe
	transcript, err := p.TranscribeAudio(ctx, audio, format)
	if err != nil { return nil, "", "", fmt.Errorf("transcription failed: %w", err) }
	if transcript == "" { return nil, "", "", fmt.Errorf("empty transcript") }

	// Step 2: Run through agent
	if p.agentChat == nil { return nil, "", transcript, fmt.Errorf("no agent chat function") }
	response, err := p.agentChat(ctx, agentID, sessionID, BuildVoiceMessagePrefix(transcript))
	if err != nil { return nil, "", transcript, fmt.Errorf("agent failed: %w", err) }

	// Step 3: Synthesize response
	audioResult, err := p.SynthesizeSpeech(ctx, response, channelType, voice)
	if err != nil {
		// TTS failed — return text response without audio
		slog.Warn("voice.pipeline.tts_failed_returning_text", "error", err)
		return nil, response, transcript, nil
	}

	return audioResult, response, transcript, nil
}

// CanTranscribe returns true if STT is available.
func (p *VoicePipeline) CanTranscribe() bool { return p.manager.HasSTT() }

// CanSynthesize returns true if TTS is available.
func (p *VoicePipeline) CanSynthesize() bool { return p.manager.HasTTS() }
