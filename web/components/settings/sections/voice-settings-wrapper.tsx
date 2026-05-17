'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useRouter } from 'next/navigation';
import { Mic, ArrowRight } from 'lucide-react';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';
import { Card, usePrefs } from './primitives';

function SliderRow({
  label, hint, min, max, step, value, format, onChange,
}: {
  label: string; hint: string;
  min: number; max: number; step: number;
  value: number; format: (v: number) => string;
  onChange: (v: number) => void;
}) {
  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <label className="text-xs font-medium">{label}</label>
        <span className="text-2xs font-mono text-muted-foreground">{format(value)}</span>
      </div>
      <input
        type="range"
        min={min} max={max} step={step}
        value={value}
        onChange={e => onChange(Number(e.target.value))}
        className="w-full accent-primary"
      />
      <p className="text-2xs text-muted-foreground mt-1">{hint}</p>
    </div>
  );
}

function VADTuner({ enabled }: { enabled: boolean }) {
  const { prefs, savePrefs } = usePrefs();

  const current = {
    positive:   Number(prefs['voice.vad.positive_threshold'] ?? 0.6),
    negative:   Number(prefs['voice.vad.negative_threshold'] ?? 0.15),
    minSpeech:  Number(prefs['voice.vad.min_speech_ms']       ?? 200),
    prePad:     Number(prefs['voice.vad.pre_speech_pad_ms']   ?? 300),
    redemption: Number(prefs['voice.vad.redemption_ms']       ?? 800),
    bargeIn:    prefs['voice.vad.barge_in'] !== false,
  };

  const update = async (patch: Record<string, unknown>) => {
    try {
      await savePrefs(patch);
      toast.success('Voice tuning saved — applies to next voice session');
    } catch {
      toast.error('Could not save voice settings. Please try again.');
    }
  };

  return (
    <Card
      title="Voice Activity Detection"
      description="Tune how the mic decides when you're speaking. Defaults work in a quiet room; raise thresholds in noisy environments."
    >
      <div className={cn('grid grid-cols-1 gap-4 md:grid-cols-2', !enabled && 'opacity-60 pointer-events-none')}>
        <SliderRow
          label="Speech threshold"
          hint="Probability to start listening. Higher = less likely to trigger on background noise."
          min={0.3} max={0.95} step={0.05}
          value={current.positive}
          format={v => v.toFixed(2)}
          onChange={v => update({ 'voice.vad.positive_threshold': v })}
        />
        <SliderRow
          label="Silence threshold"
          hint="Probability to stop listening. Keep well below the speech threshold."
          min={0.05} max={0.5} step={0.05}
          value={current.negative}
          format={v => v.toFixed(2)}
          onChange={v => update({ 'voice.vad.negative_threshold': v })}
        />
        <SliderRow
          label="Minimum utterance"
          hint="Shorter than this (ms) is dropped as a cough / door slam."
          min={100} max={800} step={50}
          value={current.minSpeech}
          format={v => `${v} ms`}
          onChange={v => update({ 'voice.vad.min_speech_ms': v })}
        />
        <SliderRow
          label="Leading pad"
          hint="Audio kept before speech start (ms) so the first syllable isn't clipped."
          min={100} max={600} step={50}
          value={current.prePad}
          format={v => `${v} ms`}
          onChange={v => update({ 'voice.vad.pre_speech_pad_ms': v })}
        />
        <SliderRow
          label="Pause tolerance"
          hint="How long to wait after you stop before ending the utterance (ms)."
          min={200} max={2000} step={100}
          value={current.redemption}
          format={v => `${v} ms`}
          onChange={v => update({ 'voice.vad.redemption_ms': v })}
        />
        <div className="flex items-start gap-3">
          <input
            type="checkbox"
            id="vad-bargein"
            checked={current.bargeIn}
            onChange={e => update({ 'voice.vad.barge_in': e.target.checked })}
            className="mt-1"
          />
          <label htmlFor="vad-bargein" className="text-xs">
            <div className="font-medium">Barge-in</div>
            <div className="text-muted-foreground">
              Stop the agent mid-reply when you start speaking. Disable if you want the agent to finish every sentence.
            </div>
          </label>
        </div>
      </div>
    </Card>
  );
}

function ProvidersLink() {
  const router = useRouter();
  return (
    <Card title="Voice Providers" description="Manage TTS, STT, and Realtime providers — add API keys, set defaults, and test connections.">
      <div className="space-y-2">
        {[
          { label: 'Text-to-Speech', desc: 'Voice replies and auto-TTS on voice channels', path: '/models-hub/tts' },
          { label: 'Speech-to-Text', desc: 'Mic transcription in the chat composer and inbound voice messages', path: '/models-hub/stt' },
        ].map(item => (
          <div key={item.path}
            className="flex items-center justify-between rounded-xl border border-border px-4 py-3">
            <div className="flex items-center gap-3">
              <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">
                <Mic className="h-4 w-4" />
              </div>
              <div>
                <p className="text-sm font-medium">{item.label}</p>
                <p className="text-xs text-muted-foreground">{item.desc}</p>
              </div>
            </div>
            <button onClick={() => router.push(item.path)}
              className="flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs font-medium hover:bg-accent transition-colors cursor-pointer">
              Manage <ArrowRight className="h-3.5 w-3.5" />
            </button>
          </div>
        ))}
      </div>
    </Card>
  );
}

export function VoiceSettingsWrapper() {
  const { prefs } = usePrefs();
  const enabled = !!prefs['services.voice'];
  return (
    <div className="space-y-6">
      <VADTuner enabled={enabled} />
      <ProvidersLink />
    </div>
  );
}
