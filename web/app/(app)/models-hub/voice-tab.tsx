'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import {
  Mic, RefreshCw, Loader2, Trash2, Star, CheckCircle2, XCircle,
  Zap, Server, X, TestTube, Plus, AlertTriangle, ArrowRight,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import {
  voice as voiceApi,
  type VoiceCatalogEntry,
  type VoiceProviderRow,
  type VoiceKind,
} from '@/lib/api';
import { toast } from 'sonner';
import { useServiceEnabled } from '@/hooks/use-service-enabled';
import { useRouter } from 'next/navigation';

export function VoiceModelsTab({ kind }: { kind: VoiceKind }) {
  const { enabled: voiceEnabled, loading: voiceLoading } = useServiceEnabled('services.voice');
  const router = useRouter();
  const [catalog, setCatalog] = useState<VoiceCatalogEntry[]>([]);
  const [rows, setRows] = useState<VoiceProviderRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [addOpen, setAddOpen] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    Promise.all([
      voiceApi.catalog().then(d => setCatalog(Array.isArray(d.drivers) ? d.drivers : [])).catch(() => {}),
      voiceApi.providers().then(d => setRows(Array.isArray(d.providers) ? d.providers : [])).catch(() => {}),
    ]).finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  const filtered = rows.filter(r => r.kind === kind);
  const filteredCatalog = catalog.filter(c => c.kind_supports.includes(kind));
  const kindLabel = kind.toUpperCase();

  return (
    <div className="space-y-5">
      {/* Disabled banner */}
      {!voiceLoading && !voiceEnabled && (
        <div className="flex items-start gap-3 rounded-xl border border-amber-400/30 bg-amber-400/5 px-4 py-3.5">
          <AlertTriangle className="h-4 w-4 text-amber-400 shrink-0 mt-0.5" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-amber-400">Voice (TTS/STT) is disabled</p>
            <p className="text-xs text-muted-foreground mt-0.5">Providers are shown but inactive. Enable Voice in Settings → Services to use them.</p>
          </div>
          <button onClick={() => router.push('/settings')}
            className="flex items-center gap-1 text-xs text-amber-400 hover:underline shrink-0">
            Settings <ArrowRight className="h-3 w-3" />
          </button>
        </div>
      )}

      {/* Catalog grid */}
      <div className={cn('rounded-xl border border-border bg-card overflow-hidden', !voiceEnabled && !voiceLoading && 'opacity-60')}>
        <div className="flex items-center justify-between px-5 py-4 border-b border-border/70 bg-muted/20">
          <div>
            <h3 className="text-sm font-semibold">{kindLabel} Drivers Catalog</h3>
            <p className="text-xs text-muted-foreground mt-0.5">Supported {kindLabel} voice drivers</p>
          </div>
          <span className="text-xs text-muted-foreground bg-muted rounded px-2 py-0.5">{filteredCatalog.length} drivers</span>
        </div>
        <div className="px-5 py-4 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {loading
            ? Array.from({ length: 6 }).map((_, i) => <div key={i} className="h-14 rounded-lg bg-muted animate-pulse" />)
            : filteredCatalog.map(entry => (
              <div key={entry.id} className="rounded-lg border border-border px-3 py-2.5 flex items-start gap-3">
                <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary/10 shrink-0">
                  {entry.hosting === 'local'
                    ? <Server className="h-4 w-4 text-primary" />
                    : <Mic className="h-4 w-4 text-primary" />}
                </div>
                <div className="min-w-0">
                  <p className="text-sm font-medium truncate">{entry.name}</p>
                  <p className="text-2xs text-muted-foreground">{entry.kind_supports.join(' · ')} · {entry.hosting}</p>
                </div>
              </div>
            ))}
        </div>
      </div>

      {/* Configured providers */}
      <div className={cn('rounded-xl border border-border bg-card overflow-hidden', !voiceEnabled && !voiceLoading && 'opacity-60 pointer-events-none select-none')}>
        <div className="flex items-center justify-between px-5 py-4 border-b border-border/70 bg-muted/20">
          <div>
            <h3 className="text-sm font-semibold">Configured {kindLabel} Providers</h3>
            <p className="text-xs text-muted-foreground mt-0.5">Active {kindLabel} providers with credentials</p>
          </div>
          <div className="flex items-center gap-2">
            <button onClick={load} className="flex h-7 w-7 items-center justify-center rounded-md hover:bg-accent">
              <RefreshCw className="h-3.5 w-3.5 text-muted-foreground" />
            </button>
            <button onClick={() => setAddOpen(true)}
              className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90">
              <Plus className="h-3.5 w-3.5" />Add provider
            </button>
          </div>
        </div>
        <div className="px-5 py-4">
          {loading ? (
            <div className="space-y-2">{Array.from({ length: 3 }).map((_, i) => <div key={i} className="h-12 rounded-lg bg-muted animate-pulse" />)}</div>
          ) : filtered.length === 0 ? (
            <div className="py-10 text-center">
              <p className="text-sm text-muted-foreground">No {kindLabel} providers configured.</p>
              <button onClick={() => setAddOpen(true)}
                className="mt-3 inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs hover:bg-accent transition-colors">
                <Plus className="h-3.5 w-3.5" />Add your first {kindLabel} provider
              </button>
            </div>
          ) : (
            <div className="space-y-2">
              {filtered.map(row => (
                <ProviderCard key={row.id} row={row} catalog={catalog} onReload={load} />
              ))}
            </div>
          )}
        </div>
      </div>

      {addOpen && (
        <AddProviderSheet
          kind={kind}
          catalog={filteredCatalog}
          onClose={() => setAddOpen(false)}
          onCreated={() => { setAddOpen(false); load(); }}
        />
      )}
    </div>
  );
}

// ─── Provider card with full CRUD ─────────────────────────────────────────

function ProviderCard({ row, catalog, onReload }: {
  row: VoiceProviderRow;
  catalog: VoiceCatalogEntry[];
  onReload: () => void;
}) {
  const entry = catalog.find(c => c.id === row.driver);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; msg: string } | null>(null);

  const doTest = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await voiceApi.testProvider(row.id);
      setTestResult(res.success
        ? { ok: true, msg: res.transcript ?? `ok — ${res.bytes ?? 0} bytes` }
        : { ok: false, msg: res.error ?? 'failed' });
    } catch (e) {
      setTestResult({ ok: false, msg: e instanceof Error ? e.message : 'Test failed' });
    } finally { setTesting(false); }
  };

  const doDefault = async () => {
    try {
      await voiceApi.setDefault(row.id);
      toast.success(`${row.name} is now the default ${row.kind.toUpperCase()} provider`);
      onReload();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed'); }
  };

  const doDelete = async () => {
    if (!confirm(`Delete "${row.name}"?`)) return;
    try {
      await voiceApi.deleteProvider(row.id);
      toast.success(`${row.name} removed`);
      onReload();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed to remove'); }
  };

  return (
    <div className="rounded-xl border border-border px-4 py-3">
      <div className="flex items-center gap-3">
        <div className={cn('flex h-8 w-8 items-center justify-center rounded-md shrink-0',
          row.kind === 'tts' ? 'bg-blue-500/10 text-blue-400' :
          row.kind === 'stt' ? 'bg-purple-500/10 text-purple-400' : 'bg-amber-500/10 text-amber-400')}>
          {entry?.hosting === 'local' ? <Server className="h-4 w-4" /> : <Mic className="h-4 w-4" />}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <p className="text-sm font-medium">{entry?.name ?? row.name ?? row.driver}</p>
            {row.is_default && (
              <span className="inline-flex items-center gap-0.5 rounded-full bg-amber-400/10 text-amber-400 text-2xs px-1.5 py-0.5">
                <Star className="h-2.5 w-2.5" />default
              </span>
            )}
          </div>
          <p className="text-2xs text-muted-foreground">{row.kind.toUpperCase()} · {entry?.hosting ?? 'cloud'}</p>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <button onClick={doTest} disabled={testing} title="Test"
            className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent">
            {testing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <TestTube className="h-3.5 w-3.5" />}
          </button>
          {!row.is_default && (
            <button onClick={doDefault} title="Set as default"
              className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-amber-400 hover:bg-accent">
              <Star className="h-3.5 w-3.5" />
            </button>
          )}
          <button onClick={doDelete} title="Remove"
            className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-destructive hover:bg-accent">
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
      {testResult && (
        <div className={cn('mt-2 text-2xs flex items-center gap-1.5 pl-11',
          testResult.ok ? 'text-emerald-400' : 'text-destructive')}>
          {testResult.ok ? <CheckCircle2 className="h-3 w-3" /> : <XCircle className="h-3 w-3" />}
          <span className="truncate">{testResult.msg}</span>
        </div>
      )}
    </div>
  );
}

// ─── Add provider sheet ───────────────────────────────────────────────────

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
  const [isDefault, setIsDefault] = useState(false);
  const [saving, setSaving] = useState(false);

  const pick = (entry: VoiceCatalogEntry) => {
    setSelected(entry);
    setName(entry.name);
    setApiKey('');
    setApiBase('');
    const s: Record<string, string> = {};
    for (const f of entry.fields) {
      if (f.name !== 'api_key' && f.name !== 'api_base') s[f.name] = '';
    }
    setSettings(s);
  };

  const save = async () => {
    if (!selected) return;
    setSaving(true);
    try {
      await voiceApi.createProvider({
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
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to add provider');
    } finally { setSaving(false); }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div className="w-full max-w-lg rounded-xl border border-border bg-card overflow-hidden flex flex-col max-h-[90vh]"
        onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-3 border-b border-border/70 bg-muted/20">
          <h3 className="text-sm font-semibold">Add {kind.toUpperCase()} provider</h3>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground">
            <X className="h-4 w-4" />
          </button>
        </div>

        {!selected ? (
          <div className="px-5 py-4 space-y-2 overflow-y-auto">
            <p className="text-xs text-muted-foreground mb-3">Cloud providers need an API key; local ones assume a running server.</p>
            {catalog.length === 0 ? (
              <p className="text-xs text-muted-foreground italic">No catalog drivers for this kind.</p>
            ) : catalog.map(entry => (
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
          </div>
        ) : (
          <div className="px-5 py-4 space-y-3 overflow-y-auto">
            <button onClick={() => setSelected(null)} className="text-2xs text-muted-foreground hover:text-foreground">← back to list</button>
            <div className="rounded-lg bg-muted/40 px-3 py-2.5 space-y-1">
              <div className="text-sm font-medium">{selected.name}</div>
              {selected.hint && <p className="text-2xs text-muted-foreground">{selected.hint}</p>}
              {selected.hardware_hint && <p className="text-2xs text-amber-400">Hardware: {selected.hardware_hint}</p>}
            </div>

            <FormField label="Display name">
              <input value={name} onChange={e => setName(e.target.value)}
                className="qr-input" />
            </FormField>

            {selected.fields.map(f => {
              const value = f.name === 'api_key' ? apiKey : f.name === 'api_base' ? apiBase : (settings[f.name] ?? '');
              const setValue = (v: string) => {
                if (f.name === 'api_key') setApiKey(v);
                else if (f.name === 'api_base') setApiBase(v);
                else setSettings(s => ({ ...s, [f.name]: v }));
              };
              return (
                <FormField key={f.name} label={f.label + (f.required ? ' *' : '')}>
                  <input type={f.type === 'password' ? 'password' : 'text'}
                    value={value}
                    onChange={e => setValue(e.target.value)}
                    placeholder={f.placeholder}
                    className="qr-input" />
                </FormField>
              );
            })}

            <label className="flex items-center gap-2 text-xs text-muted-foreground pt-1">
              <input type="checkbox" checked={isDefault} onChange={e => setIsDefault(e.target.checked)} className="rounded" />
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

function FormField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <label className="text-xs font-medium">{label}</label>
      {children}
    </div>
  );
}
