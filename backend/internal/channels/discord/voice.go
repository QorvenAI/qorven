// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package discord

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Discord voice channel integration — Soul joins a voice channel, listens to users,
// processes speech through the voice pipeline, and speaks back.
//
// Usage: User types /voice in a text channel while in a voice channel.
// The bot joins, listens, and responds via voice.

// VoiceHandler manages Discord voice connections for a guild.
type VoiceHandler struct {
	session    *discordgo.Session
	transcribe func(ctx context.Context, audio []byte, format string) (string, error)
	synthesize func(ctx context.Context, text, platform, voice string) ([]byte, error)
	chat       func(ctx context.Context, sessionID, text string) (string, error)
	mu         sync.Mutex
	conns      map[string]*voiceConn // guildID → connection
}

type voiceConn struct {
	vc     *discordgo.VoiceConnection
	cancel context.CancelFunc
}

func NewVoiceHandler(
	session *discordgo.Session,
	transcribe func(ctx context.Context, audio []byte, format string) (string, error),
	synthesize func(ctx context.Context, text, platform, voice string) ([]byte, error),
	chat func(ctx context.Context, sessionID, text string) (string, error),
) *VoiceHandler {
	return &VoiceHandler{
		session:    session,
		transcribe: transcribe,
		synthesize: synthesize,
		chat:       chat,
		conns:      make(map[string]*voiceConn),
	}
}

// JoinChannel connects the bot to a voice channel.
func (vh *VoiceHandler) JoinChannel(guildID, channelID string) error {
	vh.mu.Lock()
	defer vh.mu.Unlock()

	// Leave existing connection in this guild
	if existing, ok := vh.conns[guildID]; ok {
		existing.cancel()
		existing.vc.Disconnect()
		delete(vh.conns, guildID)
	}

	vc, err := vh.session.ChannelVoiceJoin(guildID, channelID, false, false)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	vh.conns[guildID] = &voiceConn{vc: vc, cancel: cancel}

	go vh.listenLoop(ctx, guildID, vc)
	slog.Info("discord.voice.joined", "guild", guildID, "channel", channelID)
	return nil
}

// LeaveChannel disconnects from voice in a guild.
func (vh *VoiceHandler) LeaveChannel(guildID string) {
	vh.mu.Lock()
	defer vh.mu.Unlock()

	if conn, ok := vh.conns[guildID]; ok {
		conn.cancel()
		conn.vc.Disconnect()
		delete(vh.conns, guildID)
		slog.Info("discord.voice.left", "guild", guildID)
	}
}

// listenLoop receives audio from Discord voice and processes it.
func (vh *VoiceHandler) listenLoop(ctx context.Context, guildID string, vc *discordgo.VoiceConnection) {
	vc.Speaking(true)
	defer vc.Speaking(false)

	// Discord sends Opus packets at 20ms intervals
	// Accumulate ~2 seconds of audio before processing (silence detection would be better)
	const frameDuration = 20 * time.Millisecond
	const silenceThreshold = 50 // frames of silence (~1 second)

	var audioBuf []int16
	silenceFrames := 0
	speaking := false

	for {
		select {
		case <-ctx.Done():
			return
		case pkt, ok := <-vc.OpusRecv:
			if !ok { return }

			// Decode Opus to PCM (simplified — real impl needs opus decoder)
			pcm := opusToPCM(pkt.Opus)
			if len(pcm) == 0 { continue }

			// Simple energy-based VAD
			energy := audioEnergy(pcm)
			if energy > 100 {
				speaking = true
				silenceFrames = 0
				audioBuf = append(audioBuf, pcm...)
			} else if speaking {
				silenceFrames++
				audioBuf = append(audioBuf, pcm...) // include trailing silence
				if silenceFrames >= silenceThreshold {
					// End of speech — process
					go vh.processUtterance(ctx, guildID, vc, audioBuf)
					audioBuf = nil
					speaking = false
					silenceFrames = 0
				}
			}
		}
		_ = frameDuration // used conceptually
	}
}

// processUtterance handles a complete speech segment.
func (vh *VoiceHandler) processUtterance(ctx context.Context, guildID string, vc *discordgo.VoiceConnection, pcm []int16) {
	// Convert PCM16 to bytes
	audio := make([]byte, len(pcm)*2)
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(audio[i*2:], uint16(s))
	}

	// STT
	transcript, err := vh.transcribe(ctx, audio, "pcm16")
	if err != nil || transcript == "" { return }
	slog.Info("discord.voice.heard", "guild", guildID, "text", transcript[:min(len(transcript), 60)])

	// Chat
	sessionID := "discord-voice-" + guildID
	response, err := vh.chat(ctx, sessionID, transcript)
	if err != nil { slog.Error("discord.voice.chat_error", "error", err); return }

	// TTS
	ttsAudio, err := vh.synthesize(ctx, response, "discord", "")
	if err != nil { slog.Error("discord.voice.tts_error", "error", err); return }

	// Send audio back to Discord voice channel
	vh.sendAudio(vc, ttsAudio)
}

// sendAudio sends PCM audio to a Discord voice connection as Opus frames.
func (vh *VoiceHandler) sendAudio(vc *discordgo.VoiceConnection, audio []byte) {
	// Discord expects Opus-encoded frames at 48kHz, 20ms each (960 samples)
	// Real implementation needs Opus encoder
	// For now, send raw frames — the actual encoding would use gopkg.in/hraban/opus.v2
	const frameSize = 960 * 2 * 2 // 960 samples * 2 bytes * 2 channels
	reader := io.NewSectionReader(newBytesReaderAt(audio), 0, int64(len(audio)))

	for offset := int64(0); offset < int64(len(audio)); offset += int64(frameSize) {
		frame := make([]byte, frameSize)
		n, err := reader.ReadAt(frame, offset)
		if err != nil && n == 0 { break }
		vc.OpusSend <- frame[:n]
	}
}

// --- Helpers ---

func opusToPCM(opus []byte) []int16 {
	// Placeholder — real implementation needs Opus decoder (gopkg.in/hraban/opus.v2)
	// Returns empty for now; actual decoding would produce 960 samples per 20ms frame
	return nil
}

func audioEnergy(pcm []int16) float64 {
	if len(pcm) == 0 { return 0 }
	var sum float64
	for _, s := range pcm {
		sum += float64(s) * float64(s)
	}
	return sum / float64(len(pcm))
}

type bytesReaderAt struct{ data []byte }
func newBytesReaderAt(data []byte) *bytesReaderAt { return &bytesReaderAt{data: data} }
func (b *bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b.data)) { return 0, io.EOF }
	n := copy(p, b.data[off:])
	if off+int64(n) >= int64(len(b.data)) { return n, io.EOF }
	return n, nil
}

// uses builtin min from Go 1.21+

// MaxMediaDownloadSize limits inbound media to prevent OOM.
const MaxMediaDownloadSize = 25 * 1024 * 1024 // 25MB
