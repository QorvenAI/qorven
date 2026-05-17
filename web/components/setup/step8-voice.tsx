'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { Check, CheckCircle2, Sparkles } from 'lucide-react';
import { cn } from '@/lib/utils';
import { api } from './setup-api';
import { QorvenSpinner, SectionTitle } from './setup-atoms';

type VoiceRec = { driverId: string; label: string; hint: string; needsApiKey: boolean };

const VOICE_TTS_RECOMMENDATIONS: VoiceRec[] = [
  { driverId: 'openai',     label: 'OpenAI',      hint: 'Reliable default. Bring your OpenAI key.',                             needsApiKey: true  },
  { driverId: 'elevenlabs', label: 'ElevenLabs',  hint: 'High-quality voice cloning. Bring your ElevenLabs key.',               needsApiKey: true  },
  { driverId: 'edge_tts',   label: 'Edge (free)', hint: 'Free Microsoft Edge Neural TTS. Requires edge-tts CLI on the server.', needsApiKey: false },
];

const VOICE_STT_RECOMMENDATIONS: VoiceRec[] = [
  { driverId: 'groq',           label: 'Groq Whisper',  hint: '$0.04/hr, state-of-the-art whisper-large-v3-turbo.',     needsApiKey: true  },
  { driverId: 'deepgram',       label: 'Deepgram',      hint: 'Nova-3 — real-time leader with speaker diarisation.',    needsApiKey: true  },
  { driverId: 'faster_whisper', label: 'Local whisper', hint: 'Self-hosted. Needs faster-whisper-server at :8881.',     needsApiKey: false },
];

export function Step8Voice({ onDone, onSkip }: { onDone: () => void; onSkip: () => void }) {
  const [ttsDriver, setTtsDriver] = useState<string>('');
  const [ttsKey, setTtsKey] = useState('');
  const [sttDriver, setSttDriver] = useState<string>('');
  const [sttKey, setSttKey] = useState('');
  const [saving, setSaving] = useState(false);
  const [savedCount, setSavedCount] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const pick = (rec: VoiceRec[], id: string) => rec.find((r) => r.driverId === id);

  const submit = async () => {
    setError(null);
    setSaving(true);
    let count = 0;
    try {
      if (ttsDriver) {
        const r = pick(VOICE_TTS_RECOMMENDATIONS, ttsDriver);
        if (r && (!r.needsApiKey || ttsKey)) {
          await api('/v1/voice/providers', {
            method: 'POST',
            body: JSON.stringify({ name: r.label, kind: 'tts', driver: ttsDriver, api_key: ttsKey, settings: {}, enabled: true, is_default: true }),
          });
          count += 1;
        }
      }
      if (sttDriver) {
        const r = pick(VOICE_STT_RECOMMENDATIONS, sttDriver);
        if (r && (!r.needsApiKey || sttKey)) {
          await api('/v1/voice/providers', {
            method: 'POST',
            body: JSON.stringify({ name: r.label, kind: 'stt', driver: sttDriver, api_key: sttKey, settings: {}, enabled: true, is_default: true }),
          });
          count += 1;
        }
      }
      setSavedCount(count);
      if (count > 0) setTimeout(onDone, 400);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Voice setup failed');
    } finally {
      setSaving(false);
    }
  };

  const ttsRec = pick(VOICE_TTS_RECOMMENDATIONS, ttsDriver);
  const sttRec = pick(VOICE_STT_RECOMMENDATIONS, sttDriver);

  return (
    <div className="space-y-4">
      <SectionTitle icon={Sparkles} title="Voice (optional)"
        subtitle="Wire one TTS and one STT provider so Prime can talk and listen. You can add more in Settings → Voice." />

      <div>
        <div className="mb-2 text-sm font-medium text-muted-foreground">Text-to-Speech</div>
        <div className="grid grid-cols-3 gap-2">
          {VOICE_TTS_RECOMMENDATIONS.map((r) => (
            <button key={r.driverId} onClick={() => setTtsDriver(r.driverId === ttsDriver ? '' : r.driverId)}
              className={cn('rounded-lg border px-3 py-2.5 text-left transition-colors',
                ttsDriver === r.driverId ? 'border-primary bg-primary/10' : 'border-border hover:border-border/70')}>
              <div className="text-xs font-medium">{r.label}</div>
              <p className="text-xs text-muted-foreground mt-0.5 line-clamp-2">{r.hint}</p>
            </button>
          ))}
        </div>
        {ttsRec?.needsApiKey && (
          <input type="password" value={ttsKey} onChange={(e) => setTtsKey(e.target.value)}
            placeholder={`${ttsRec.label} API key`}
            className="qr-input" />
        )}
      </div>

      <div>
        <div className="mb-2 text-sm font-medium text-muted-foreground">Speech-to-Text</div>
        <div className="grid grid-cols-3 gap-2">
          {VOICE_STT_RECOMMENDATIONS.map((r) => (
            <button key={r.driverId} onClick={() => setSttDriver(r.driverId === sttDriver ? '' : r.driverId)}
              className={cn('rounded-lg border px-3 py-2.5 text-left transition-colors',
                sttDriver === r.driverId ? 'border-primary bg-primary/10' : 'border-border hover:border-border/70')}>
              <div className="text-xs font-medium">{r.label}</div>
              <p className="text-xs text-muted-foreground mt-0.5 line-clamp-2">{r.hint}</p>
            </button>
          ))}
        </div>
        {sttRec?.needsApiKey && (
          <input type="password" value={sttKey} onChange={(e) => setSttKey(e.target.value)}
            placeholder={`${sttRec.label} API key`}
            className="qr-input" />
        )}
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">{error}</div>
      )}
      {savedCount > 0 && (
        <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-400 flex items-center gap-1.5">
          <CheckCircle2 className="h-3.5 w-3.5" />
          {savedCount} voice provider{savedCount === 1 ? '' : 's'} configured — moving on…
        </div>
      )}

      <div className="flex items-center justify-between gap-2 pt-1">
        <button onClick={onSkip} className="text-xs text-muted-foreground hover:text-foreground">
          Skip — configure later
        </button>
        <button onClick={submit} disabled={saving || (!ttsDriver && !sttDriver)}
          className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40 cursor-pointer">
          {saving ? <QorvenSpinner className="h-3.5 w-3.5" /> : <Check className="h-3.5 w-3.5" />}
          Save &amp; continue
        </button>
      </div>
    </div>
  );
}
