// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"nhooyr.io/websocket"
)

// Plivo voice call integration — alternative to Twilio for phone calls.
// Uses Plivo's Audio Streaming for real-time audio over WebSocket.
//
// Flow:
//   Inbound call → Plivo → XML (stream to WebSocket) → Qorven → STT → Agent → TTS → Plivo → Caller

type PlivoConfig struct {
	AuthID    string
	AuthToken string
	FromNumber string
	WebhookURL string
}

type PlivoVoice struct {
	cfg      PlivoConfig
	pipeline *VoicePipeline
	client   *http.Client
}

func NewPlivoVoice(cfg PlivoConfig, pipeline *VoicePipeline) *PlivoVoice {
	return &PlivoVoice{cfg: cfg, pipeline: pipeline, client: &http.Client{}}
}

// HandleInboundCall returns Plivo XML that connects the call to a WebSocket stream.
func (pv *PlivoVoice) HandleInboundCall(w http.ResponseWriter, r *http.Request) {
	from := r.FormValue("From")
	slog.Info("plivo.inbound_call", "from", from)

	wsURL := pv.cfg.WebhookURL + "/v1/voice/plivo/stream"
	wsURL = "wss" + wsURL[len("https"):]

	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Speak>Hello, you are connected to a Qorven AI assistant.</Speak>
  <Stream bidirectional="true" keepCallAlive="true" contentType="audio/x-l16;rate=8000">%s</Stream>
</Response>`, wsURL)

	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(xml))
}

// HandleMediaStream handles the WebSocket connection from Plivo Audio Streaming.
func (pv *PlivoVoice) HandleMediaStream(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil { slog.Error("plivo.ws.accept_failed", "error", err); return }
	defer conn.Close(websocket.StatusNormalClosure, "")

	session := &plivoStreamSession{conn: conn, pipeline: pv.pipeline, audioBuf: &bytes.Buffer{}}
	session.run(r.Context())
}

type plivoStreamSession struct {
	conn      *websocket.Conn
	pipeline  *VoicePipeline
	streamID  string
	audioBuf  *bytes.Buffer
}

func (s *plivoStreamSession) run(ctx context.Context) {
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil { return }

		var event plivoEvent
		json.Unmarshal(data, &event)

		switch event.Event {
		case "start":
			s.streamID = event.Start.StreamID
			slog.Info("plivo.stream.started", "stream", s.streamID)

		case "media":
			audio, err := base64.StdEncoding.DecodeString(event.Media.Payload)
			if err != nil {
				slog.Warn("plivo.media.decode_failed", "err", err)
				continue
			}
			s.audioBuf.Write(audio)
			if s.audioBuf.Len() >= 16000 { // ~1 second at 16kHz L16
				chunk := make([]byte, s.audioBuf.Len())
				copy(chunk, s.audioBuf.Bytes())
				s.audioBuf.Reset()
				go s.processAudio(ctx, chunk)
			}

		case "stop":
			slog.Info("plivo.stream.stopped", "stream", s.streamID)
			return
		}
	}
}

func (s *plivoStreamSession) processAudio(ctx context.Context, audio []byte) {
	if s.pipeline == nil { return }

	transcript, err := s.pipeline.TranscribeAudio(ctx, audio, "pcm16")
	if err != nil || transcript == "" { return }

	result, err := s.pipeline.SynthesizeSpeech(ctx, transcript, "phone", "")
	if err != nil { return }

	// Send audio back via Plivo playAudio JSON event
	encoded := base64.StdEncoding.EncodeToString(result.Audio)
	msg, _ := json.Marshal(map[string]any{
		"event": "playAudio",
		"media": map[string]string{"payload": encoded},
	})
	s.conn.Write(ctx, websocket.MessageText, msg)
}

// MakeCall initiates an outbound phone call.
func (pv *PlivoVoice) MakeCall(ctx context.Context, toNumber string) (string, error) {
	payload := map[string]string{
		"from":       pv.cfg.FromNumber,
		"to":         toNumber,
		"answer_url": pv.cfg.WebhookURL + "/v1/voice/plivo/inbound",
	}
	body, _ := json.Marshal(payload)

	apiURL := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/Call/", pv.cfg.AuthID)
	req, _ := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	req.SetBasicAuth(pv.cfg.AuthID, pv.cfg.AuthToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := pv.client.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()

	var result struct{ RequestUUID string `json:"request_uuid"` }
	respBody, _ := io.ReadAll(resp.Body)
	json.Unmarshal(respBody, &result)
	if resp.StatusCode >= 400 { return "", fmt.Errorf("plivo call: %s", string(respBody)) }

	slog.Info("plivo.call.initiated", "to", toNumber, "uuid", result.RequestUUID)
	return result.RequestUUID, nil
}

type plivoEvent struct {
	Event string `json:"event"`
	Start struct {
		StreamID string `json:"streamId"`
	} `json:"start"`
	Media struct {
		Payload string `json:"payload"` // base64-encoded audio
	} `json:"media"`
}
