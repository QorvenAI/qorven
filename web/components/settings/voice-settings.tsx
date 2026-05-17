'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Mic, Volume2, Radio, Loader2, Plus, Trash2, Star, CheckCircle2, XCircle,
  Zap, Server, X, TestTube,
} from 'lucide-react';
import { toast } from 'sonner';

import { cn } from '@/lib/utils';
import {
  voice,
  type VoiceCatalogEntry,
  type VoiceKind,
  type VoiceProviderRow,
} from '@/lib/api';

// ─── Settings → Voice ──────────────────────────────────────────────────
//
// Catalog-driven CRUD for TTS/STT/Realtime providers. Renders three
// sections stacked. Each section:
//   - Lists every enabled row for that kind
//   - Shows a default-pinning star + test + delete actions inline
//   - Has an "Add provider" button that opens a catalog picker → form
//
// The form fields come straight from the catalog's `fields` array so
// adding a new driver in voice_catalog.json automatically surfaces
// here without UI code changes.

interface VoiceSettingsProps {
  /** Whether Settings → Services → Voice toggle is on. When false
   * we render an informational banner and disable the controls. */
  enabled: boolean;
}

export function VoiceSettings({ enabled }: VoiceSettingsProps) {
  const [catalog, setCatalog] = useState<VoiceCatalogEntry[]>([]);
  const [rows, setRows] = useState<VoiceProviderRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [addKind, setAddKind] = useState<VoiceKind | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [cat, providers] = await Promise.all([
        voice.catalog(),
        voice.providers(),
      ]);
      setCatalog(cat.drivers ?? []);
      setRows(providers.providers ?? []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Voice load failed');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const byKind = useMemo(() => ({
    tts: rows.filter((r) => r.kind === 'tts'),
    stt: rows.filter((r) => r.kind === 'stt'),
    realtime: rows.filter((r) => r.kind === 'realtime'),
  }), [rows]);

  if (loading) {
    return <div className="flex items-center gap-2 text-sm text-muted-foreground p-6"><Loader2 className="h-4 w-4 animate-spin" />Loading voice providers…</div>;
  }

  return (
    <div className="space-y-5">
      {!enabled && (
        <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-xs text-amber-200">
          Voice is globally disabled in <strong>Services</strong>. Providers you configure here will be saved but won&apos;t run until you flip the toggle back on.
        </div>
      )}

      <KindSection
        kind="tts"
        title="Text-to-Speech"
        description="Used for Prime&apos;s voice replies + auto-TTS on voice channels (Telegram voice notes, etc.)."
        icon={Volume2}
        catalog={catalog}
        rows={byKind.tts}
        onAdd={() => setAddKind('tts')}
        onReload={load}
      />

      <KindSection
        kind="stt"
        title="Speech-to-Text"
        description="Transcribes the mic button in the chat composer + inbound voice messages."
        icon={Mic}
        catalog={catalog}
        rows={byKind.stt}
        onAdd={() => setAddKind('stt')}
        onReload={load}
      />

      <KindSection
        kind="realtime"
        title="Realtime (full-duplex)"
        description="Optional. Used by the Prime floating voice bar for voice-to-voice without a transcribe/synthesise round-trip."
        icon={Radio}
        catalog={catalog}
        rows={byKind.realtime}
        onAdd={() => setAddKind('realtime')}
        onReload={load}
      />

      {addKind && (
        <AddProviderSheet
          kind={addKind}
          catalog={catalog.filter((c) => c.kind_supports.includes(addKind))}
          onClose={() => setAddKind(null)}
          onCreated={() => { setAddKind(null); load(); }}
        />
      )}
    </div>
  );
}

// ─── One section per kind ───────────────────────────────────────────────

function KindSection(props: {
  kind: VoiceKind;
  title: string;
  description: string;
  icon: React.ElementType;
  catalog: VoiceCatalogEntry[];
  rows: VoiceProviderRow[];
  onAdd: () => void;
  onReload: () => void;
}) {
  const { title, description, icon: Icon, rows, onAdd, onReload, catalog } = props;
  const catalogFor = (id: string) => catalog.find((c) => c.id === id);

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="flex items-start justify-between px-6 py-4 border-b border-border/70 bg-muted/20">
        <div className="flex items-start gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">
            <Icon className="h-4 w-4" />
          </div>
          <div>
            <h3 className="text-sm font-semibold">{title}</h3>
            <p className="text-xs text-muted-foreground mt-0.5">{description}</p>
          </div>
        </div>
        <button onClick={onAdd}
          className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90">
          <Plus className="h-3.5 w-3.5" />Add provider
        </button>
      </div>
      <div className="px-6 py-5 space-y-2">
        {rows.length === 0 ? (
          <div className="text-xs text-muted-foreground italic">No providers configured yet.</div>
        ) : rows.map((r) => (
          <ProviderCard key={r.id} row={r} entry={catalogFor(r.driver)} onReload={onReload} />
        ))}
      </div>
    </div>
  );
}

// ─── One provider card ──────────────────────────────────────────────────

function ProviderCard({ row, entry, onReload }: {
  row: VoiceProviderRow;
  entry: VoiceCatalogEntry | undefined;
  onReload: () => void;
}) {
  const [testing, setTesting] = useState(false);
  const [lastTest, setLastTest] = useState<{ ok: boolean; msg: string } | null>(null);

  const doTest = async () => {
    setTesting(true);
    try {
      const res = await voice.testProvider(row.id);
      if (res.success) {
        setLastTest({ ok: true, msg: res.transcript ?? `ok — ${res.bytes ?? 0} bytes` });
      } else {
        setLastTest({ ok: false, msg: res.error ?? 'failed' });
      }
    } catch (err) {
      setLastTest({ ok: false, msg: err instanceof Error ? err.message : String(err) });
    } finally {
      setTesting(false);
    }
  };

  const doDefault = async () => {
    try {
      await voice.setDefault(row.id);
      toast.success(`${row.name} is now the default ${row.kind.toUpperCase()} provider`);
      onReload();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to set default');
    }
  };

  const doDelete = async () => {
    if (!confirm(`Delete "${row.name}"?`)) return;
    try {
      await voice.deleteProvider(row.id);
      toast.success(`${row.name} removed`);
      onReload();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete');
    }
  };

  return (
    <div className="rounded-lg border border-border/70 bg-muted/20 px-4 py-2.5">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <div className="flex h-7 w-7 items-center justify-center rounded-md bg-background border border-border">
            {entry?.hosting === 'local' ? <Server className="h-3.5 w-3.5 text-muted-foreground" /> : <Zap className="h-3.5 w-3.5 text-muted-foreground" />}
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium truncate">{row.name}</span>
              {row.is_default && (
                <span className="inline-flex items-center gap-0.5 rounded-full bg-amber-400/10 text-amber-400 text-2xs px-1.5 py-0.5"><Star className="h-2.5 w-2.5" />default</span>
              )}
              {!row.enabled && (
                <span className="rounded-full bg-muted text-muted-foreground text-2xs px-1.5 py-0.5">disabled</span>
              )}
            </div>
            <p className="text-2xs text-muted-foreground truncate">{entry?.name ?? row.driver} · {row.kind.toUpperCase()}</p>
          </div>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <button onClick={doTest} disabled={testing}
            title="Test this provider"
            className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent">
            {testing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <TestTube className="h-3.5 w-3.5" />}
          </button>
          {!row.is_default && (
            <button onClick={doDefault}
              title="Make default"
              className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-amber-400 hover:bg-accent">
              <Star className="h-3.5 w-3.5" />
            </button>
          )}
          <button onClick={doDelete}
            title="Remove"
            className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-destructive hover:bg-accent">
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
      {lastTest && (
        <div className={cn('mt-2 text-2xs flex items-center gap-1.5',
          lastTest.ok ? 'text-emerald-400' : 'text-destructive')}>
          {lastTest.ok ? <CheckCircle2 className="h-3 w-3" /> : <XCircle className="h-3 w-3" />}
          <span className="truncate">{lastTest.msg}</span>
        </div>
      )}
    </div>
  );
}

// ─── Add-provider sheet ────────────────────────────────────────────────

function AddProviderSheet({ kind, catalog, onClose, onCreated }: {
  kind: VoiceKind;
  catalog: VoiceCatalogEntry[];
  onClose: () => void;
  onCreated: () => void;
}) {
  const [selected, setSelected] = useState<VoiceCatalogEntry | null>(null);
  const [name, setName] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [apiBase, setApiBase] = useState('');
  const [settings, setSettings] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [isDefault, setIsDefault] = useState(false);

  const pick = (entry: VoiceCatalogEntry) => {
    setSelected(entry);
    setName(entry.name);
    setApiBase('');
    setApiKey('');
    // Pre-populate common settings keys.
    const s: Record<string, string> = {};
    for (const f of entry.fields) {
      if (f.name !== 'api_key' && f.name !== 'api_base') {
        s[f.name] = '';
      }
    }
    setSettings(s);
  };

  const save = async () => {
    if (!selected) return;
    setSaving(true);
    try {
      await voice.createProvider({
        name: name || selected.name,
        kind,
        driver: selected.id,
        api_base: apiBase,
        api_key: apiKey,
        settings,
        enabled: true,
        is_default: isDefault,
      });
      toast.success(`${selected.name} added`);
      onCreated();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add provider');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div className="w-full max-w-lg rounded-xl border border-border bg-card overflow-hidden flex flex-col max-h-[90vh]"
        onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-3 border-b border-border/70">
          <h3 className="text-sm font-semibold">Add {kind.toUpperCase()} provider</h3>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground">
            <X className="h-4 w-4" />
          </button>
        </div>

        {!selected ? (
          // Step 1 — pick a driver from the catalog.
          <div className="px-5 py-4 space-y-2 overflow-y-auto">
            <p className="text-xs text-muted-foreground">Pick a provider. Cloud providers need an API key; local ones assume a running server.</p>
            {catalog.map((entry) => (
              <button key={entry.id} onClick={() => pick(entry)}
                className="w-full text-left rounded-lg border border-border bg-background/50 hover:border-primary/40 hover:bg-primary/5 px-3 py-2.5 transition-colors">
                <div className="flex items-start gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-md bg-muted shrink-0 mt-0.5">
                    {entry.hosting === 'local' ? <Server className="h-3.5 w-3.5" /> : <Zap className="h-3.5 w-3.5" />}
                  </div>
                  <div className="min-w-0">
                    <div className="text-sm font-medium">{entry.name}</div>
                    {entry.hint && <p className="text-2xs text-muted-foreground mt-0.5 line-clamp-2">{entry.hint}</p>}
                  </div>
                </div>
              </button>
            ))}
            {catalog.length === 0 && (
              <div className="text-xs text-muted-foreground italic">No catalog drivers for this kind.</div>
            )}
          </div>
        ) : (
          // Step 2 — fill in the driver's fields.
          <div className="px-5 py-4 space-y-3 overflow-y-auto">
            <button onClick={() => setSelected(null)} className="text-2xs text-muted-foreground hover:text-foreground">← back to list</button>
            <div className="rounded-lg bg-muted/40 px-3 py-2.5 space-y-1">
              <div className="text-sm font-medium">{selected.name}</div>
              {selected.hint && <p className="text-2xs text-muted-foreground">{selected.hint}</p>}
              {selected.hardware_hint && (
                <p className="text-2xs text-amber-400">Hardware: {selected.hardware_hint}</p>
              )}
            </div>

            <Field label="Display name">
              <input value={name} onChange={(e) => setName(e.target.value)}
                className="qr-input" />
            </Field>

            {selected.fields.map((f) => {
              const value = f.name === 'api_key' ? apiKey :
                            f.name === 'api_base' ? apiBase :
                            (settings[f.name] ?? '');
              const setValue = (v: string) => {
                if (f.name === 'api_key')  setApiKey(v);
                else if (f.name === 'api_base') setApiBase(v);
                else setSettings((s) => ({ ...s, [f.name]: v }));
              };
              return (
                <Field key={f.name} label={f.label + (f.required ? ' *' : '')}>
                  <input type={f.type === 'password' ? 'password' : 'text'}
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                    placeholder={f.placeholder}
                    className="qr-input" />
                </Field>
              );
            })}

            <label className="flex items-center gap-2 text-xs text-muted-foreground pt-1">
              <input type="checkbox" checked={isDefault}
                onChange={(e) => setIsDefault(e.target.checked)}
                className="rounded" />
              Make this the default {kind.toUpperCase()} provider
            </label>
          </div>
        )}

        {selected && (
          <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-border/70 bg-muted/20">
            <button onClick={onClose} className="rounded-lg border border-input px-3 py-1.5 text-xs hover:bg-accent">Cancel</button>
            <button onClick={save} disabled={saving}
              className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50">
              {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
              Add provider
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <label className="text-xs font-medium">{label}</label>
      {children}
    </div>
  );
}
