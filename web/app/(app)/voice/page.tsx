'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import {
  Mic, MicOff, Volume2, Play, Loader2, AlertCircle,
  Radio, Save, Settings, Plus, Trash2, Star,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import {
  voice,
  type VoiceConfig,
  type VoiceProviders,
  type VoiceProviderRow,
  type VoiceCatalogEntry,
  type VoiceCatalogField,
} from '@/lib/api';
import { useVoiceEnabled } from '@/hooks/use-voice-enabled';
import Link from 'next/link';

export default function VoicePage() {
  const { enabled: voiceEnabled, loading: voiceLoading } = useVoiceEnabled();
  const [providers, setProviders] = useState<VoiceProviders | null>(null);
  const [providerRows, setProviderRows] = useState<VoiceProviderRow[]>([]);
  const [cfg, setCfg] = useState<VoiceConfig | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const fetchProviders = () => {
    voice.providers()
      .then((p) => {
        const m = p?.manager ?? {};
        setProviders({
          tts: Array.isArray(m.tts) ? m.tts : [],
          stt: Array.isArray(m.stt) ? m.stt : [],
          primary_tts: m.primary_tts,
          primary_stt: m.primary_stt,
          auto: m.auto,
        });
        setProviderRows(Array.isArray(p?.providers) ? p.providers : []);
      })
      .catch(() => { setProviders({ tts: [], stt: [] }); setProviderRows([]); });
  };

  useEffect(() => {
    fetchProviders();
    voice.config().then(setCfg).catch(() => setCfg({}));
  }, []);

  return (
    <div className="mx-auto max-w-4xl space-y-5 p-4 lg:p-6">
      <header>
        <h1 className="flex items-center gap-2 text-lg font-semibold">
          <Radio className="h-6 w-6 text-primary" />
          Voice
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Manage TTS + STT providers. Recording requires microphone permission.
        </p>
      </header>

      {!voiceLoading && !voiceEnabled && (
        <div className="flex items-start gap-3 rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 text-xs text-amber-300">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <div>
            Voice is currently disabled in your account settings. Turn it on in{' '}
            <Link href="/settings" className="underline hover:text-amber-200">
              Settings → Services
            </Link>{' '}
            to use the testers below.
          </div>
        </div>
      )}

      {err && (
        <div className="flex items-center gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          <AlertCircle className="h-4 w-4" />
          <span>{err}</span>
        </div>
      )}

      <ProvidersManager
        rows={providerRows}
        providers={providers}
        cfg={cfg}
        onRefresh={fetchProviders}
        onError={setErr}
      />

      <TtsTester defaultVoice={cfg?.openai?.voice ?? cfg?.edge?.voice} onError={setErr} />
      <SttTester onError={setErr} />

      <ConfigEditor cfg={cfg} onSave={setCfg} onError={setErr} />
    </div>
  );
}

// ───────────────────────────────────────────────────────────────────

function ProvidersManager({
  rows,
  providers,
  cfg,
  onRefresh,
  onError,
}: {
  rows: VoiceProviderRow[];
  providers: VoiceProviders | null;
  cfg: VoiceConfig | null;
  onRefresh: () => void;
  onError: (m: string | null) => void;
}) {
  const [showAdd, setShowAdd] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [settingDefault, setSettingDefault] = useState<string | null>(null);

  const deleteRow = async (id: string) => {
    setDeleting(id);
    onError(null);
    try {
      await voice.deleteProvider(id);
      onRefresh();
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Delete failed');
    } finally {
      setDeleting(null);
    }
  };

  const setDefault = async (id: string) => {
    setSettingDefault(id);
    onError(null);
    try {
      await voice.setDefault(id);
      onRefresh();
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Failed to set default');
    } finally {
      setSettingDefault(null);
    }
  };

  if (!providers) {
    return (
      <div className="flex items-center gap-2 rounded-xl border border-border bg-card/40 p-4 text-2xs text-muted-foreground">
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
        Loading providers…
      </div>
    );
  }

  return (
    <section className="rounded-xl border border-border bg-card/40">
      <header className="flex items-center justify-between border-b border-border/60 px-3 py-2">
        <div className="flex items-center gap-2">
          <Radio className="h-3.5 w-3.5 text-muted-foreground" />
          <h2 className="text-xs font-semibold tracking-wider">PROVIDERS</h2>
          <span className="text-2xs text-muted-foreground">{rows.length} configured</span>
        </div>
        <button
          onClick={() => setShowAdd((v) => !v)}
          className="inline-flex items-center gap-1 rounded-md border border-border px-2 py-1 text-2xs hover:bg-accent"
        >
          <Plus className="h-3 w-3" />
          {showAdd ? 'Cancel' : 'Add Provider'}
        </button>
      </header>

      {showAdd && (
        <AddProviderForm
          onAdded={() => { setShowAdd(false); onRefresh(); }}
          onError={onError}
        />
      )}

      {rows.length === 0 && !showAdd ? (
        <p className="p-4 text-2xs text-muted-foreground">
          No providers configured. Click Add Provider to connect a TTS or STT service.
        </p>
      ) : (
        <ul className="divide-y divide-border/60">
          {rows.map((row) => (
            <li key={row.id} className="flex items-center gap-3 px-3 py-2 text-xs">
              <div className={cn(
                'flex h-5 w-5 shrink-0 items-center justify-center rounded-full',
                row.enabled ? 'bg-emerald-500/15' : 'bg-muted',
              )}>
                {row.kind === 'tts' ? (
                  <Volume2 className={cn('h-3 w-3', row.enabled ? 'text-emerald-500' : 'text-muted-foreground')} />
                ) : (
                  <Mic className={cn('h-3 w-3', row.enabled ? 'text-emerald-500' : 'text-muted-foreground')} />
                )}
              </div>
              <div className="min-w-0 flex-1">
                <span className="font-medium">{row.name}</span>
                <span className="ml-2 font-mono text-2xs text-muted-foreground">{row.driver}</span>
              </div>
              <span className={cn(
                'rounded-sm px-1.5 py-0.5 font-mono text-2xs uppercase',
                row.kind === 'tts' ? 'bg-blue-500/10 text-blue-400' : 'bg-purple-500/10 text-purple-400',
              )}>{row.kind}</span>
              {row.is_default && (
                <span className="rounded-sm bg-primary/15 px-1.5 py-0.5 font-mono text-2xs uppercase text-primary">default</span>
              )}
              {!row.is_default && (
                <button
                  onClick={() => setDefault(row.id)}
                  disabled={settingDefault === row.id}
                  title="Set as default"
                  className="text-muted-foreground hover:text-foreground disabled:opacity-50"
                >
                  {settingDefault === row.id
                    ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    : <Star className="h-3.5 w-3.5" />}
                </button>
              )}
              <button
                onClick={() => deleteRow(row.id)}
                disabled={deleting === row.id}
                className="text-destructive hover:text-red-400 disabled:opacity-50"
              >
                {deleting === row.id
                  ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  : <Trash2 className="h-3.5 w-3.5" />}
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function AddProviderForm({
  onAdded,
  onError,
}: {
  onAdded: () => void;
  onError: (m: string | null) => void;
}) {
  const [catalog, setCatalog] = useState<VoiceCatalogEntry[] | null>(null);
  const [selectedDriver, setSelectedDriver] = useState('');
  const [kind, setKind] = useState<'tts' | 'stt'>('tts');
  const [fieldValues, setFieldValues] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    voice.catalog()
      .then((r) => {
        const drivers = r?.drivers ?? [];
        setCatalog(drivers);
        if (drivers.length > 0) setSelectedDriver(drivers[0]!.id);
      })
      .catch(() => setCatalog([]));
  }, []);

  const entry = catalog?.find((c) => c.id === selectedDriver);

  const submit = async () => {
    if (!selectedDriver || !entry) return;
    setBusy(true);
    onError(null);
    try {
      const settings: Record<string, unknown> = {};
      for (const f of entry.fields) {
        if (fieldValues[f.name]) settings[f.name] = fieldValues[f.name];
      }
      await voice.createProvider({
        driver: selectedDriver,
        name: entry.name,
        kind,
        settings,
        enabled: true,
      });
      onAdded();
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Failed to add provider');
    } finally {
      setBusy(false);
    }
  };

  if (!catalog) {
    return (
      <div className="flex items-center gap-2 border-b border-border/60 p-3 text-2xs text-muted-foreground">
        <Loader2 className="h-3 w-3 animate-spin" /> Loading catalog…
      </div>
    );
  }

  if (catalog.length === 0) {
    return (
      <div className="border-b border-border/60 p-3 text-2xs text-muted-foreground">
        No drivers available in catalog.
      </div>
    );
  }

  return (
    <div className="space-y-3 border-b border-border/60 bg-muted/20 p-3">
      <div className="flex flex-wrap gap-2">
        <div className="flex flex-col gap-1">
          <label className="text-2xs text-muted-foreground">Driver</label>
          <select
            value={selectedDriver}
            onChange={(e) => { setSelectedDriver(e.target.value); setFieldValues({}); }}
            className="qr-select text-xs"
          >
            {catalog.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
        </div>
        <div className="flex flex-col gap-1">
          <label className="text-2xs text-muted-foreground">Kind</label>
          <select
            value={kind}
            onChange={(e) => setKind(e.target.value as 'tts' | 'stt')}
            className="qr-select text-xs"
          >
            {(entry?.kind_supports ?? ['tts', 'stt']).includes('tts') && <option value="tts">TTS</option>}
            {(entry?.kind_supports ?? ['tts', 'stt']).includes('stt') && <option value="stt">STT</option>}
          </select>
        </div>
        {entry?.fields.map((f: VoiceCatalogField) => (
          <div key={f.name} className="flex flex-col gap-1">
            <label className="text-2xs text-muted-foreground">
              {f.label}{f.required && <span className="text-destructive"> *</span>}
            </label>
            <input
              type={f.type === 'password' ? 'password' : 'text'}
              value={fieldValues[f.name] ?? ''}
              onChange={(e) => setFieldValues((v) => ({ ...v, [f.name]: e.target.value }))}
              placeholder={f.placeholder ?? f.name}
              className="qr-input text-xs" />
          </div>
        ))}
      </div>
      {entry?.hint && (
        <p className="text-2xs text-muted-foreground">{entry.hint}</p>
      )}
      <button
        onClick={submit}
        disabled={busy}
        className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
      >
        {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
        Add
      </button>
    </div>
  );
}

// ───────────────────────────────────────────────────────────────────

function TtsTester({
  defaultVoice,
  onError,
}: {
  defaultVoice?: string;
  onError: (m: string | null) => void;
}) {
  const [text, setText] = useState('Hello from Qorven. Voice synthesis is working.');
  const [voiceId, setVoiceId] = useState(defaultVoice ?? '');
  const [model, setModel] = useState('');
  const [busy, setBusy] = useState(false);
  const [audioUrl, setAudioUrl] = useState<string | null>(null);

  useEffect(() => () => { if (audioUrl) URL.revokeObjectURL(audioUrl); }, [audioUrl]);

  const speak = async () => {
    if (!text.trim() || busy) return;
    setBusy(true);
    onError(null);
    try {
      const blob = await voice.speech({
        input: text,
        voice: voiceId.trim() || undefined,
        model: model.trim() || undefined,
      });
      const url = URL.createObjectURL(blob);
      if (audioUrl) URL.revokeObjectURL(audioUrl);
      setAudioUrl(url);
      // Auto-play — users expect it after clicking Play.
      const audio = new Audio(url);
      audio.play().catch(() => {
        // Autoplay blocked — the <audio> element below lets them play manually.
      });
    } catch (e) {
      onError(e instanceof Error ? e.message : 'TTS failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="rounded-xl border border-border bg-card/40">
      <header className="flex items-center gap-2 border-b border-border/60 px-3 py-2">
        <Volume2 className="h-3.5 w-3.5 text-muted-foreground" />
        <h2 className="text-xs font-semibold tracking-wider">SPEAK (TTS)</h2>
      </header>
      <div className="space-y-2 p-3">
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          rows={3}
          className="qr-textarea resize-none" />
        <div className="flex flex-wrap gap-2">
          <input
            value={voiceId}
            onChange={(e) => setVoiceId(e.target.value)}
            placeholder="voice (optional — e.g. alloy, en-US-AriaNeural)"
            className="qr-select flex-1 min-w-[180px] text-xs"
          />
          <input
            value={model}
            onChange={(e) => setModel(e.target.value)}
            placeholder="model (optional)"
            className="qr-select flex-1 min-w-[140px] text-xs"
          />
          <button
            onClick={speak}
            disabled={busy || !text.trim()}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
            Speak
          </button>
        </div>
        {audioUrl && (
          <audio controls src={audioUrl} className="mt-1 w-full" />
        )}
      </div>
    </section>
  );
}

// ───────────────────────────────────────────────────────────────────

function SttTester({ onError }: { onError: (m: string | null) => void }) {
  const [recording, setRecording] = useState(false);
  const [transcript, setTranscript] = useState('');
  const [busy, setBusy] = useState(false);
  const mediaRec = useRef<MediaRecorder | null>(null);
  const chunks = useRef<BlobPart[]>([]);

  useEffect(() => () => { mediaRec.current?.stop(); }, []);

  const start = async () => {
    if (recording) return;
    onError(null);
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const mime = MediaRecorder.isTypeSupported('audio/webm') ? 'audio/webm' : '';
      const rec = new MediaRecorder(stream, mime ? { mimeType: mime } : undefined);
      chunks.current = [];
      rec.ondataavailable = (e) => { if (e.data.size > 0) chunks.current.push(e.data); };
      rec.onstop = async () => {
        stream.getTracks().forEach((t) => t.stop());
        const blob = new Blob(chunks.current, { type: mime || 'audio/webm' });
        setBusy(true);
        try {
          const res = await voice.transcribe(blob, 'recording.webm');
          setTranscript(res.text ?? '');
        } catch (e) {
          onError(e instanceof Error ? e.message : 'Transcription failed');
        } finally {
          setBusy(false);
        }
      };
      rec.start();
      mediaRec.current = rec;
      setRecording(true);
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Microphone unavailable');
    }
  };

  const stop = () => {
    if (mediaRec.current?.state === 'recording') {
      mediaRec.current.stop();
    }
    setRecording(false);
  };

  return (
    <section className="rounded-xl border border-border bg-card/40">
      <header className="flex items-center gap-2 border-b border-border/60 px-3 py-2">
        <Mic className="h-3.5 w-3.5 text-muted-foreground" />
        <h2 className="text-xs font-semibold tracking-wider">TRANSCRIBE (STT)</h2>
      </header>
      <div className="space-y-2 p-3">
        <div className="flex items-center gap-2">
          {!recording ? (
            <button
              onClick={start}
              disabled={busy}
              className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card px-3 py-1 text-xs hover:bg-accent disabled:opacity-50"
            >
              <Mic className="h-3.5 w-3.5" />
              Record
            </button>
          ) : (
            <button
              onClick={stop}
              className="inline-flex items-center gap-1.5 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-1 text-xs text-destructive hover:bg-destructive/20"
            >
              <MicOff className="h-3.5 w-3.5" />
              Stop & transcribe
            </button>
          )}
          {recording && (
            <span className="flex items-center gap-1 text-2xs text-destructive">
              <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-destructive" />
              Recording…
            </span>
          )}
          {busy && (
            <span className="flex items-center gap-1 text-2xs text-muted-foreground">
              <Loader2 className="h-3 w-3 animate-spin" /> Transcribing…
            </span>
          )}
        </div>
        {transcript && (
          <div className="rounded-md border border-border bg-background p-3 text-sm leading-relaxed">
            {transcript}
          </div>
        )}
        {!transcript && !recording && !busy && (
          <p className="text-2xs text-muted-foreground">
            Click Record, speak, then Stop & transcribe. Browser asks for mic permission the first time.
          </p>
        )}
      </div>
    </section>
  );
}

// ───────────────────────────────────────────────────────────────────

function ConfigEditor({
  cfg,
  onSave,
  onError,
}: {
  cfg: VoiceConfig | null;
  onSave: (c: VoiceConfig) => void;
  onError: (m: string | null) => void;
}) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<string>('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (open && cfg) setDraft(JSON.stringify(cfg, null, 2));
  }, [open, cfg]);

  const save = async () => {
    setBusy(true);
    onError(null);
    try {
      const parsed = JSON.parse(draft) as VoiceConfig;
      const next = await voice.saveConfig(parsed);
      onSave(next);
      setOpen(false);
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Invalid JSON or save failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="rounded-xl border border-border bg-card/40">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 border-b border-border/60 px-3 py-2 text-left hover:bg-accent/40"
      >
        <Settings className="h-3.5 w-3.5 text-muted-foreground" />
        <h2 className="flex-1 text-xs font-semibold tracking-wider">CONFIG (advanced)</h2>
        <span className="text-2xs text-muted-foreground">{open ? 'close' : 'open'}</span>
      </button>
      {open && (
        <div className="space-y-2 p-3">
          <p className="text-2xs text-muted-foreground">
            Raw JSON. Fields vary per provider (openai.voice, elevenlabs.voice_id,
            edge.voice, whisper.model, etc.). Touch carefully.
          </p>
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            rows={14}
            className="qr-textarea resize-y"
          />
          <button
            onClick={save}
            disabled={busy}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
            Save
          </button>
        </div>
      )}
    </section>
  );
}
