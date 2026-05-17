'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// useVoice — end-to-end voice chat hook.
//
// The flow on every utterance:
//   1. VAD (Silero ONNX) fires onSpeechStart  → we show "listening" UI
//   2. User stops talking → VAD fires onSpeechEnd with a Float32Array
//      of the PCM samples
//   3. We encode WAV, base64-encode, and send via WebSocket to /ws/voice
//   4. Backend runs STT → agent → TTS; streams "thinking" / "response" /
//      "audio" events back
//   5. We play the TTS audio. If the user starts speaking again mid-
//      playback (detected by VAD), we stop the audio immediately —
//      "barge-in" — so the conversation feels human.
//
// VAD assets are served from /vad/ (public/vad/) and the ONNX runtime
// WASM from /onnx/ (public/onnx/). No CDN dependency, works offline,
// and doesn't break if jsdelivr has an outage.
//
// Tunables are exposed as props with sensible defaults. Overriding the
// defaults is what "voice feels right for this user" looks like — some
// people speak in short bursts with long pauses, others the opposite.

import { useCallback, useEffect, useRef, useState } from 'react';
import { useMicVAD, utils } from '@ricky0123/vad-react';
import { userPrefs } from '@/lib/api';
import { wsBase } from '@/lib/api-url';

const WS_VOICE_URL = typeof window !== 'undefined'
  ? wsBase('/api/ws/voice')
  : '';

export type VoiceState = 'idle' | 'listening' | 'processing' | 'speaking';

// VADOptions tuning reference (all milliseconds unless noted):
//
//   positiveSpeechThreshold:
//     Probability at or above which a frame counts as speech. Higher
//     = fewer false starts on background noise. Default 0.6 works
//     for a quiet office; bump to 0.75+ for a café or open-plan.
//
//   negativeSpeechThreshold:
//     Probability below which a frame counts as silence. Keep this
//     well under `positiveSpeechThreshold` — the gap is the hysteresis
//     band that stops the VAD flapping on breath pauses.
//
//   minSpeechMs:
//     Minimum utterance length before we bother sending it upstream.
//     Anything shorter is probably a cough or door slam. 200ms is a
//     decent floor — a single syllable ("no") fits.
//
//   preSpeechPadMs:
//     How much audio BEFORE the detected speech start to include in
//     the captured clip. Crucial for STT accuracy: without padding,
//     the front consonant gets clipped ("ello" instead of "hello").
//
//   redemptionMs:
//     How long we keep listening after the VAD thinks speech ended,
//     in case the user pauses mid-sentence. Higher = fewer premature
//     cutoffs but slower response. 800ms is a good natural-pause feel.
export interface VADOptions {
  positiveSpeechThreshold?: number;
  negativeSpeechThreshold?: number;
  minSpeechMs?: number;
  preSpeechPadMs?: number;
  redemptionMs?: number;
}

interface UseVoiceOptions {
  agentId: string;
  onTranscript?: (text: string) => void;
  onResponse?: (text: string) => void;
  onError?: (error: string) => void;
  // Barge-in: when true (default), user speech during TTS playback
  // cancels the playback so the agent stops talking the moment the
  // user starts. Disable when recording voicemail-style clips where
  // the agent shouldn't be interrupted.
  bargeIn?: boolean;
  vad?: VADOptions;
}

const DEFAULT_VAD: Required<VADOptions> = {
  positiveSpeechThreshold: 0.6,
  negativeSpeechThreshold: 0.15,
  minSpeechMs: 200,
  preSpeechPadMs: 300,
  redemptionMs: 800,
};

export function useVoice({
  agentId,
  onTranscript,
  onResponse,
  onError,
  bargeIn: bargeInProp,
  vad: vadOpts,
}: UseVoiceOptions) {
  const [state, setState] = useState<VoiceState>('idle');
  const [volume, setVolume] = useState(0);
  const [active, setActive] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  // currentAudioRef holds the <Audio> element that's playing TTS so we
  // can stop it when the user interrupts. Without this reference,
  // barge-in would have nothing to pause.
  const currentAudioRef = useRef<HTMLAudioElement | null>(null);

  // Merge order: DEFAULT_VAD < user prefs < explicit props.
  // Props override prefs so callers can force a specific tuning for
  // e.g. a walkie-talkie UI. User prefs let the end user tune voice
  // from Settings once, and every surface (Prime widget, chat dictate,
  // /voice tester) picks it up.
  const [prefsVAD, setPrefsVAD] = useState<Required<VADOptions>>(DEFAULT_VAD);
  const [prefsBargeIn, setPrefsBargeIn] = useState(true);

  useEffect(() => {
    // Async load prefs once per mount. Silent failure: if the endpoint
    // isn't reachable we just stick with DEFAULT_VAD. VAD still works;
    // it just won't reflect the user's tuning until the next mount.
    let cancelled = false;
    userPrefs.get().then((p) => {
      if (cancelled || !p) return;
      setPrefsVAD({
        positiveSpeechThreshold: Number(p['voice.vad.positive_threshold'] ?? DEFAULT_VAD.positiveSpeechThreshold),
        negativeSpeechThreshold: Number(p['voice.vad.negative_threshold'] ?? DEFAULT_VAD.negativeSpeechThreshold),
        minSpeechMs:             Number(p['voice.vad.min_speech_ms']       ?? DEFAULT_VAD.minSpeechMs),
        preSpeechPadMs:          Number(p['voice.vad.pre_speech_pad_ms']   ?? DEFAULT_VAD.preSpeechPadMs),
        redemptionMs:            Number(p['voice.vad.redemption_ms']       ?? DEFAULT_VAD.redemptionMs),
      });
      setPrefsBargeIn(p['voice.vad.barge_in'] !== false);
    }).catch(() => {});
    return () => { cancelled = true; };
  }, []);

  const vadCfg = { ...DEFAULT_VAD, ...prefsVAD, ...(vadOpts ?? {}) };
  const bargeIn = bargeInProp ?? prefsBargeIn;

  // Connect WebSocket. Lazy: we only open when start() is called so
  // the widget doesn't hold a socket open for the whole page lifetime.
  const getWs = useCallback((): Promise<WebSocket> => {
    return new Promise((resolve, reject) => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        resolve(wsRef.current);
        return;
      }
      wsRef.current?.close();
      const ws = new WebSocket(`${WS_VOICE_URL}?agent_id=${agentId}`);
      let connectTimer: ReturnType<typeof setTimeout> | null = null;
      ws.onopen = () => {
        if (connectTimer) { clearTimeout(connectTimer); connectTimer = null; }
        wsRef.current = ws;
        resolve(ws);
      };
      ws.onmessage = (e) => {
        try {
          const evt = JSON.parse(e.data);
          switch (evt.type) {
            case 'transcript':
              onTranscript?.(evt.data ?? '');
              break;

            case 'thinking':
              setState('processing');
              break;

            case 'response':
              onResponse?.(evt.data ?? '');
              // Stay in "processing" until the audio arrives — flipping to
              // "listening" here would reopen the mic while TTS is about
              // to play, causing immediate self-barge-in.
              break;

            case 'audio':
              if (evt.audio_base64) {
                setState('speaking');
                // Clean up any prior audio before starting a new one.
                if (currentAudioRef.current) {
                  currentAudioRef.current.pause();
                  currentAudioRef.current.src = '';
                }
                const bytes = Uint8Array.from(atob(evt.audio_base64), (c) => c.charCodeAt(0));
                const blob = new Blob([bytes], { type: 'audio/' + (evt.format || 'wav') });
                const url = URL.createObjectURL(blob);
                const el = new Audio(url);
                currentAudioRef.current = el;
                el.onended = () => {
                  URL.revokeObjectURL(url);
                  if (currentAudioRef.current === el) {
                    currentAudioRef.current = null;
                  }
                  setState('listening');
                };
                el.onerror = () => {
                  URL.revokeObjectURL(url);
                  setState('listening');
                };
                el.play().catch(() => setState('listening'));
              } else {
                setState('listening');
              }
              break;

            case 'redirect':
              // Long responses redirect to the agent's full chat page
              // (better than synthesizing a wall of text).
              if (typeof window !== 'undefined') {
                window.dispatchEvent(
                  new CustomEvent('voice-redirect', { detail: { agentId: evt.data } }),
                );
              }
              break;

            case 'error':
              onError?.(evt.data ?? 'Voice error');
              setState('listening');
              break;
          }
        } catch {
          /* malformed event — ignore */
        }
      };
      ws.onerror = () => {
        if (connectTimer) { clearTimeout(connectTimer); connectTimer = null; }
        onError?.('Connection failed');
        reject();
      };
      ws.onclose = () => {
        if (wsRef.current === ws) wsRef.current = null;
      };
      // Give the handshake a bounded window — 5s is generous on
      // localhost, still reasonable over typical WAN.
      connectTimer = setTimeout(() => {
        connectTimer = null;
        if (ws.readyState !== WebSocket.OPEN) {
          ws.close();
          reject();
        }
      }, 5000);
    });
  }, [agentId, onTranscript, onResponse, onError]);

  // VAD hook — always mounted but paused when not active. Always-
  // mounted means no ONNX model reload between sessions (costs ~1s).
  //
  // Asset paths point at our own /vad/ and /onnx/ instead of jsdelivr.
  // Files are committed under web/public/vad and web/public/onnx.
  const vad = useMicVAD({
    startOnLoad: false,
    baseAssetPath: '/vad/',
    onnxWASMBasePath: '/onnx/',
    positiveSpeechThreshold: vadCfg.positiveSpeechThreshold,
    negativeSpeechThreshold: vadCfg.negativeSpeechThreshold,
    minSpeechMs: vadCfg.minSpeechMs,
    preSpeechPadMs: vadCfg.preSpeechPadMs,
    redemptionMs: vadCfg.redemptionMs,
    onSpeechStart: () => {
      setVolume(0.6);
      // Barge-in: user started talking over the agent's TTS → cut it.
      // Without this, the user's voice gets mixed with the agent's
      // own voice on the upstream audio, which produces garbage STT.
      if (bargeIn && currentAudioRef.current) {
        currentAudioRef.current.pause();
        currentAudioRef.current.currentTime = 0;
        currentAudioRef.current = null;
        setState('listening');
        // Let the backend know so it can abort any in-flight TTS
        // synth / cancel the remaining agent response.
        wsRef.current?.send(JSON.stringify({ type: 'interrupt' }));
      }
    },
    onSpeechEnd: async (audio: Float32Array) => {
      setVolume(0);
      setState('processing');
      // Convert to WAV (the backend's STT drivers all accept WAV —
      // no codec negotiation). WAV is fatter than Opus but the
      // utterances are short and the speedup of skipping transcode
      // on the client is worth it.
      const wavBuffer = utils.encodeWAV(audio);
      const wavBlob = new Blob([wavBuffer], { type: 'audio/wav' });
      const reader = new FileReader();
      reader.onloadend = () => {
        const base64 = (reader.result as string).split(',')[1];
        if (wsRef.current?.readyState === WebSocket.OPEN) {
          wsRef.current.send(
            JSON.stringify({ type: 'audio', audio_base64: base64, format: 'wav' }),
          );
        }
      };
      reader.readAsDataURL(wavBlob);
    },
  });

  // Cleanup on unmount — close sockets + stop any in-flight audio.
  useEffect(() => {
    return () => {
      wsRef.current?.close();
      wsRef.current = null;
      if (currentAudioRef.current) {
        currentAudioRef.current.pause();
        currentAudioRef.current.src = '';
        currentAudioRef.current = null;
      }
    };
  }, []);

  const start = useCallback(async () => {
    try {
      await getWs();
      await vad.start();
      setActive(true);
      setState('listening');
    } catch {
      onError?.('Failed to start voice');
    }
  }, [getWs, vad, onError]);

  const stop = useCallback(async () => {
    await vad.pause();
    wsRef.current?.close();
    wsRef.current = null;
    if (currentAudioRef.current) {
      currentAudioRef.current.pause();
      currentAudioRef.current.src = '';
      currentAudioRef.current = null;
    }
    setActive(false);
    setState('idle');
    setVolume(0);
  }, [vad]);

  const toggle = useCallback(async () => {
    if (active) { await stop(); } else { await start(); }
  }, [active, start, stop]);

  return { state, volume, toggle, start, stop };
}

