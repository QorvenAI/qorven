// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
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
	"net/url"
	"sync"

	"nhooyr.io/websocket"
)

// Twilio voice call integration — Souls can make and receive phone calls.
// Uses Twilio's Media Streams for real-time audio over WebSocket.
//
// Flow:
//   Inbound call → Twilio → TwiML (connect to WebSocket) → Qorven → STT → Agent → TTS → Twilio → Caller
//   Outbound call → Qorven → Twilio REST API → Twilio calls user → same WebSocket flow

type TwilioConfig struct {
	AccountSID string
	AuthToken  string
	FromNumber string // Twilio phone number
	WebhookURL string // public URL for Twilio to connect to
}

type TwilioVoice struct {
	cfg      TwilioConfig
	pipeline *VoicePipeline
	client   *http.Client
}

func NewTwilioVoice(cfg TwilioConfig, pipeline *VoicePipeline) *TwilioVoice {
	return &TwilioVoice{cfg: cfg, pipeline: pipeline, client: &http.Client{}}
}

// --- Inbound Call Handler ---

// HandleInboundCall returns TwiML that connects the call to a WebSocket for media streaming.
// Mount this at your webhook URL (e.g. POST /v1/voice/twilio/inbound)
func (tv *TwilioVoice) HandleInboundCall(w http.ResponseWriter, r *http.Request) {
	from := r.FormValue("From")
	to := r.FormValue("To")
	slog.Info("twilio.inbound_call", "from", from, "to", to)

	// Return TwiML that connects to our WebSocket for media streaming
	wsURL := tv.cfg.WebhookURL + "/v1/voice/twilio/stream"
	wsURL = "wss" + wsURL[len("https"):] // convert https to wss

	twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Say>Hello, you are connected to a Qorven AI assistant.</Say>
  <Connect>
    <Stream url="%s">
      <Parameter name="from" value="%s"/>
    </Stream>
  </Connect>
</Response>`, wsURL, from)

	w.Header().Set("Content-Type", "text/xml")
	w.Write([]byte(twiml))
}

// HandleMediaStream handles the WebSocket connection from Twilio Media Streams.
// Mount this at /v1/voice/twilio/stream
func (tv *TwilioVoice) HandleMediaStream(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil { slog.Error("twilio.ws.accept_failed", "error", err); return }
	defer conn.Close(websocket.StatusNormalClosure, "")

	session := &twilioStreamSession{
		conn:     conn,
		pipeline: tv.pipeline,
		audioBuf: &bytes.Buffer{},
	}
	session.run(r.Context())
}

type twilioStreamSession struct {
	conn       *websocket.Conn
	pipeline   *VoicePipeline
	streamSID  string
	callSID    string
	from       string
	audioBuf   *bytes.Buffer
	mu         sync.Mutex // guards audioBuf
	wmu        sync.Mutex // guards conn.Write
}

func (s *twilioStreamSession) run(ctx context.Context) {
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil { return }

		var event twilioEvent
		json.Unmarshal(data, &event)

		switch event.Event {
		case "connected":
			slog.Info("twilio.stream.connected")

		case "start":
			s.streamSID = event.StreamSID
			s.callSID = event.Start.CallSID
			if params, ok := event.Start.CustomParameters["from"]; ok {
				s.from = params
			}
			slog.Info("twilio.stream.started", "stream", s.streamSID, "call", s.callSID)

		case "media":
			// Decode mulaw audio from Twilio
			audio, _ := base64.StdEncoding.DecodeString(event.Media.Payload)
			s.mu.Lock()
			s.audioBuf.Write(audio)
			// Process when we have enough audio (~1 second at 8kHz mulaw = 8000 bytes)
			if s.audioBuf.Len() >= 8000 {
				audioData := make([]byte, s.audioBuf.Len())
				copy(audioData, s.audioBuf.Bytes())
				s.audioBuf.Reset()
				s.mu.Unlock()

				// Process through voice pipeline
				go s.processAudio(ctx, audioData)
			} else {
				s.mu.Unlock()
			}

		case "stop":
			slog.Info("twilio.stream.stopped", "stream", s.streamSID)
			return
		}
	}
}

func (s *twilioStreamSession) processAudio(ctx context.Context, audio []byte) {
	if s.pipeline == nil { return }

	// Transcribe the audio
	transcript, err := s.pipeline.TranscribeAudio(ctx, audio, "mulaw")
	if err != nil || transcript == "" { return }

	slog.Info("twilio.transcribed", "text", transcript[:min(len(transcript), 60)])

	// Synthesize response
	result, err := s.pipeline.SynthesizeSpeech(ctx, transcript, "phone", "")
	if err != nil { return }

	// Send audio back to Twilio via WebSocket (serialized — nhooyr.io/websocket panics on concurrent writes)
	encoded := base64.StdEncoding.EncodeToString(result.Audio)
	msg := []byte(fmt.Sprintf(`{"event":"media","streamSid":"%s","media":{"payload":"%s"}}`, s.streamSID, encoded))
	s.wmu.Lock()
	s.conn.Write(ctx, websocket.MessageText, msg)
	s.wmu.Unlock()
}

// --- Outbound Calls ---

// MakeCall initiates an outbound phone call to a number.
func (tv *TwilioVoice) MakeCall(ctx context.Context, toNumber string) (string, error) {
	data := url.Values{
		"To":   {toNumber},
		"From": {tv.cfg.FromNumber},
		"Url":  {tv.cfg.WebhookURL + "/v1/voice/twilio/inbound"}, // same TwiML handler
	}
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls.json", tv.cfg.AccountSID)
	req, _ := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBufferString(data.Encode()))
	req.SetBasicAuth(tv.cfg.AccountSID, tv.cfg.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := tv.client.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()

	var result struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
	}
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &result)
	if resp.StatusCode >= 400 { return "", fmt.Errorf("twilio call: %s", string(body)) }

	slog.Info("twilio.call.initiated", "to", toNumber, "sid", result.SID)
	return result.SID, nil
}

// --- Types ---

type twilioEvent struct {
	Event     string `json:"event"`
	StreamSID string `json:"streamSid"`
	Start     struct {
		CallSID          string            `json:"callSid"`
		CustomParameters map[string]string `json:"customParameters"`
	} `json:"start"`
	Media struct {
		Payload string `json:"payload"` // base64 mulaw audio
	} `json:"media"`
}

// uses builtin min from Go 1.21+
