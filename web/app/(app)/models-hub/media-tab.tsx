'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useCallback } from 'react';
import { Plus, Trash2, Loader2, X, CheckCircle2, AlertCircle, Zap, Key, ImageIcon, Film, Music } from 'lucide-react';
import { cn } from '@/lib/utils';
import { mediaProviders, type MediaCatalogEntry, type MediaProviderRow, type MediaKind } from '@/lib/api';
import { toast } from 'sonner';

import type { LucideIcon } from 'lucide-react';

const MEDIA_KIND_META: Record<MediaKind, { title: string; description: string; Icon: LucideIcon }> = {
  image:     { title: 'Image Generation', description: 'DALL-E 3, Stability AI, FLUX — generates images from text prompts. Used by the create_image tool.', Icon: ImageIcon },
  video:     { title: 'Video Generation', description: 'Sora, Runway, Kling — generates videos from text. Used by the create_video tool (coming soon).', Icon: Film },
  audio_gen: { title: 'Audio Generation', description: 'Music, sound effects, and ambient audio generation.', Icon: Music },
};

export function MediaTab({ kind }: { kind: MediaKind }) {
  const [catalog, setCatalog] = useState<MediaCatalogEntry[]>([]);
  const [rows, setRows] = useState<MediaProviderRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [adding, setAdding] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [cat, prov] = await Promise.all([mediaProviders.catalog(), mediaProviders.list()]);
      setCatalog(cat.drivers ?? []);
      setRows(prov.providers ?? []);
    } catch { /* ignore */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const meta = MEDIA_KIND_META[kind];

  if (loading) return <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" />Loading media providers…</div>;

  return (
    <div className="space-y-5">
      <MediaKindSection
        kind={kind}
        title={meta.title}
        description={meta.description}
        icon={<meta.Icon className="h-4 w-4" />}
        catalog={catalog.filter(c => c.kind === kind)}
        rows={rows.filter(r => r.kind === kind)}
        onAdd={() => setAdding(true)}
        onReload={load}
      />
      {adding && (
        <AddMediaProviderSheet
          kind={kind}
          catalog={catalog.filter(c => c.kind === kind)}
          onClose={() => setAdding(false)}
          onCreated={() => { setAdding(false); load(); }}
        />
      )}
    </div>
  );
}

function MediaKindSection({ kind, title, description, icon, catalog, rows, onAdd, onReload }: {
  kind: MediaKind; title: string; description: string; icon: React.ReactNode;
  catalog: MediaCatalogEntry[]; rows: MediaProviderRow[]; onAdd: () => void; onReload: () => void;
}) {
  const catalogFor = (id: string) => catalog.find(c => c.id === id);
  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="flex items-start justify-between px-6 py-4 border-b border-border/70 bg-muted/20">
        <div className="flex items-start gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">{icon}</div>
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
          <div className="text-xs text-muted-foreground italic">No {kind} providers configured yet.</div>
        ) : rows.map(r => (
          <MediaProviderCard key={r.id} row={r} entry={catalogFor(r.driver)} onReload={onReload} />
        ))}
      </div>
    </div>
  );
}

function MediaProviderCard({ row, entry, onReload }: { row: MediaProviderRow; entry: MediaCatalogEntry | undefined; onReload: () => void }) {
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; msg: string } | null>(null);

  const doTest = async () => {
    setTesting(true);
    try {
      const res = await mediaProviders.test(row.id);
      setTestResult(res.success
        ? { ok: true, msg: res.url ? `ok — ${res.url.substring(0, 60)}…` : `ok — ${res.bytes ?? 0} bytes` }
        : { ok: false, msg: res.error ?? 'failed' });
    } catch (e) { setTestResult({ ok: false, msg: e instanceof Error ? e.message : String(e) }); }
    finally { setTesting(false); }
  };

  const doDefault = async () => {
    try {
      await mediaProviders.setDefault(row.id, row.kind);
      toast.success(`${row.name} is now the default ${row.kind} provider`);
      onReload();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed'); }
  };

  const doDelete = async () => {
    if (!confirm(`Delete "${row.name}"?`)) return;
    try {
      await mediaProviders.delete(row.id);
      toast.success('Provider removed');
      onReload();
    } catch { toast.error('Failed to remove'); }
  };

  return (
    <div className="rounded-lg border border-border/70 bg-muted/20 px-4 py-2.5">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <div className="flex h-7 w-7 items-center justify-center rounded-md bg-background border border-border text-primary">
            {row.kind === 'image' ? <ImageIcon className="h-3.5 w-3.5" /> : row.kind === 'video' ? <Film className="h-3.5 w-3.5" /> : <Music className="h-3.5 w-3.5" />}
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium truncate">{row.name}</span>
              {row.is_default && (
                <span className="inline-flex items-center gap-0.5 rounded-full bg-amber-400/10 text-amber-400 text-2xs px-1.5 py-0.5">
                  <CheckCircle2 className="h-2.5 w-2.5" />default
                </span>
              )}
            </div>
            <p className="text-2xs text-muted-foreground truncate">{entry?.name ?? row.driver} · {row.kind}</p>
          </div>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <button onClick={doTest} disabled={testing} title="Test" className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent">
            {testing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Zap className="h-3.5 w-3.5" />}
          </button>
          {!row.is_default && (
            <button onClick={doDefault} title="Set as default" className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-amber-400 hover:bg-accent">
              <Key className="h-3.5 w-3.5" />
            </button>
          )}
          <button onClick={doDelete} title="Remove" className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-destructive hover:bg-accent">
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
      {testResult && (
        <div className={cn('mt-2 text-2xs flex items-center gap-1.5', testResult.ok ? 'text-emerald-400' : 'text-destructive')}>
          {testResult.ok ? <CheckCircle2 className="h-3 w-3" /> : <AlertCircle className="h-3 w-3" />}
          <span className="truncate">{testResult.msg}</span>
        </div>
      )}
    </div>
  );
}

function AddMediaProviderSheet({ kind, catalog, onClose, onCreated }: {
  kind: MediaKind; catalog: MediaCatalogEntry[]; onClose: () => void; onCreated: () => void;
}) {
  const [selected, setSelected] = useState<MediaCatalogEntry | null>(null);
  const [name, setName] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [apiBase, setApiBase] = useState('');
  const [settings, setSettings] = useState<Record<string, string>>({});
  const [isDefault, setIsDefault] = useState(false);
  const [saving, setSaving] = useState(false);

  const pick = (e: MediaCatalogEntry) => {
    setSelected(e);
    setName(e.name);
    setApiBase(e.default_base ?? '');
    setApiKey('');
    const s: Record<string, string> = {};
    for (const f of e.fields) {
      if (f.name !== 'api_key' && f.name !== 'api_base') s[f.name] = '';
    }
    setSettings(s);
  };

  const save = async () => {
    if (!selected) return;
    setSaving(true);
    try {
      await mediaProviders.create({
        name: name || selected.name,
        kind,
        driver: selected.id,
        api_base: apiBase,
        api_key: apiKey,
        settings,
        enabled: true,
        is_default: isDefault,
        fallback_order: 0,
      });
      toast.success(`${selected.name} added`);
      onCreated();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed to add provider'); }
    finally { setSaving(false); }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div className="w-full max-w-lg rounded-xl border border-border bg-card overflow-hidden flex flex-col max-h-[90vh]" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-3 border-b border-border/70">
          <h3 className="text-sm font-semibold">Add {kind} provider</h3>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground"><X className="h-4 w-4" /></button>
        </div>
        {!selected ? (
          <div className="px-5 py-4 space-y-2 overflow-y-auto">
            <p className="text-xs text-muted-foreground">Select a provider to configure.</p>
            {catalog.map(e => (
              <button key={e.id} onClick={() => pick(e)}
                className="w-full text-left rounded-lg border border-border bg-background/50 hover:border-primary/40 hover:bg-primary/5 px-3 py-2.5 transition-colors">
                <div className="text-sm font-medium">{e.name}</div>
                {e.hint && <p className="text-2xs text-muted-foreground mt-0.5">{e.hint}</p>}
                {e.models && <p className="text-2xs text-muted-foreground/60 mt-0.5">Models: {e.models.slice(0, 3).join(', ')}</p>}
              </button>
            ))}
            {catalog.length === 0 && <div className="text-xs text-muted-foreground italic">No drivers available for {kind} yet.</div>}
          </div>
        ) : (
          <div className="px-5 py-4 space-y-3 overflow-y-auto">
            <button onClick={() => setSelected(null)} className="text-2xs text-muted-foreground hover:text-foreground">← back</button>
            <div className="rounded-lg bg-muted/40 px-3 py-2.5">
              <div className="text-sm font-medium">{selected.name}</div>
              {selected.hint && <p className="text-2xs text-muted-foreground mt-0.5">{selected.hint}</p>}
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium">Display name</label>
              <input value={name} onChange={e => setName(e.target.value)}
                className="qr-input" />
            </div>
            {selected.fields.map(f => {
              const value = f.name === 'api_key' ? apiKey : f.name === 'api_base' ? apiBase : (settings[f.name] ?? '');
              const setValue = (v: string) => {
                if (f.name === 'api_key') setApiKey(v);
                else if (f.name === 'api_base') setApiBase(v);
                else setSettings(s => ({ ...s, [f.name]: v }));
              };
              return (
                <div key={f.name} className="space-y-1">
                  <label className="text-xs font-medium">{f.label}{f.required ? ' *' : ''}</label>
                  <input type={f.type === 'password' ? 'password' : 'text'}
                    value={value} onChange={e => setValue(e.target.value)} placeholder={f.placeholder}
                    className="qr-input" />
                </div>
              );
            })}
            {selected.models && selected.models.length > 0 && (
              <div className="rounded-lg bg-muted/30 px-3 py-2 text-2xs text-muted-foreground">
                Available models: {selected.models.join(' · ')}
              </div>
            )}
            <label className="flex items-center gap-2 text-xs text-muted-foreground pt-1">
              <input type="checkbox" checked={isDefault} onChange={e => setIsDefault(e.target.checked)} className="rounded" />
              Set as default {kind} provider
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
