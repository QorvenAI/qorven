// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ─── Piper TTS (local static binary) ───────────────────────────────────
//
// Piper is a single-binary CPU TTS from Rhasspy. Voice models are
// small (~20-60 MB per voice) and the binary runs well on a
// Raspberry Pi. We shell out to the `piper` CLI rather than call
// C++ via cgo — keeps the Go build portable, and matches how the
// existing EdgeTTS driver works.
//
// Usage:
//   echo "text" | piper --model /path/to/voice.onnx --output_file out.wav
//
// Voice files live outside the repo; the user points
// settings.model_path at their download. Install hint surfaced in
// the catalog's hardware_hint field.

type PiperTTS struct {
	binary    string
	modelPath string
	// Optional: user-supplied piper binary path. Defaults to "piper"
	// on PATH (what a `sudo apt install piper-tts` or homebrew gives
	// you).
}

// NewPiperTTS returns a Piper TTS adapter. modelPath points at a
// .onnx voice file; binary is optional (falls back to "piper" on PATH).
func NewPiperTTS(binary, modelPath string) *PiperTTS {
	if binary == "" { binary = "piper" }
	return &PiperTTS{binary: binary, modelPath: modelPath}
}

func (p *PiperTTS) Name() string { return "piper" }

func (p *PiperTTS) Synthesize(ctx context.Context, text string, opts TTSOptions) (*AudioResult, error) {
	if p.modelPath == "" {
		return nil, fmt.Errorf("piper tts: voice model path required (settings.model_path)")
	}

	// Piper reads text on stdin and writes WAV to stdout when you
	// pass --output_file - (dash). Keeps us from juggling temp
	// files just to return bytes.
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(tctx, p.binary,
		"--model", p.modelPath,
		"--output_file", "-",
	)
	cmd.Stdin = strings.NewReader(text)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("piper tts: %w (stderr: %s) — install: https://github.com/rhasspy/piper",
			err, strings.TrimSpace(stderr.String()))
	}
	return &AudioResult{
		Audio:     stdout.Bytes(),
		Extension: "wav",
		MimeType:  "audio/wav",
	}, nil
}
