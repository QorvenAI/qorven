'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { providers as providersApi } from '@/lib/api';
import { BrandIcon } from '@/components/brand-icon';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import { X, Plus, CheckCircle, AlertCircle, Clock, Shield, BarChart3, Cpu, Eye, EyeOff } from 'lucide-react';

interface Props {
  provider: any; // catalog manifest
  onClose: () => void;
  onConnected: () => void;
}

type Tab = 'keys' | 'models' | 'usage';

export function ProviderDetailPanel({ provider, onClose, onConnected }: Props) {
  const [tab, setTab] = useState<Tab>('keys');
  const [keys, setKeys] = useState<any[]>([]);
  const [models, setModels] = useState<any[]>([]);
  const [usage, setUsage] = useState<any[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [defaultModel, setDefaultModel] = useState<string>('');
  const [showAddKey, setShowAddKey] = useState(false);
  const [newLabel, setNewLabel] = useState('');
  const [newKey, setNewKey] = useState('');
  const [showKey, setShowKey] = useState(false);
  const [saving, setSaving] = useState(false);
  const [verifying, setVerifying] = useState<string | null>(null);
  const [modelsError, setModelsError] = useState(false);

  const hasVerifiedKey = keys.some((k) => k.status === 'verified');

  useEffect(() => {
    providersApi.listKeys(provider.id).then((d) => setKeys(Array.isArray(d) ? d : [])).catch(() => {});
  }, [provider.id]);

  useEffect(() => {
    if (tab === 'models' && hasVerifiedKey) {
      setModelsError(false);
      providersApi.liveModels(provider.id).then((d: any) => {
        const m = d?.models || provider.models || [];
        // Sort: latest/preview first, then alphabetical
        m.sort((a: any, b: any) => {
          const aName = typeof a === 'string' ? a : a.name || a.id;
          const bName = typeof b === 'string' ? b : b.name || b.id;
          const aPreview = aName.includes('preview') || aName.includes('3.') || aName.includes('3-') ? 0 : 1;
          const bPreview = bName.includes('preview') || bName.includes('3.') || bName.includes('3-') ? 0 : 1;
          if (aPreview !== bPreview) return aPreview - bPreview;
          return bName.localeCompare(aName); // reverse alpha = newest first
        });
        setModels(m);
      }).catch(() => { setModelsError(true); setModels([]); });
      // Load selected
      providersApi.selectedModels(provider.id).then((d: any) => {
        const sel = new Set<string>();
        let def = '';
        (Array.isArray(d) ? d : []).forEach((s: any) => { sel.add(s.model_id); if (s.is_default) def = s.model_id; });
        setSelected(sel);
        setDefaultModel(def);
      }).catch(() => {});
    }
    if (tab === 'usage') {
      providersApi.usage(provider.id).then((d) => setUsage(Array.isArray(d) ? d : [])).catch(() => {});
    }
  }, [tab, hasVerifiedKey, provider]);

  const handleAddKey = async () => {
    if (!newKey) return;
    setSaving(true);
    try {
      await providersApi.addKey(provider.id, { label: newLabel || 'Key ' + (keys.length + 1), key: newKey });
      const updated = await providersApi.listKeys(provider.id);
      setKeys(Array.isArray(updated) ? updated : []);
      setShowAddKey(false); setNewLabel(''); setNewKey('');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add key');
    } finally {
      setSaving(false);
    }
  };

  const handleVerify = async (keyId: string) => {
    setVerifying(keyId);
    try {
      await providersApi.verifyKey(keyId);
      const updated = await providersApi.listKeys(provider.id);
      setKeys(Array.isArray(updated) ? updated : []);
      onConnected();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Verification failed');
    } finally {
      setVerifying(null);
    }
  };

  const handleRetire = async (keyId: string) => {
    if (!confirm('Remove this key? Usage history will be preserved.')) return;
    try {
      await providersApi.retireKey(keyId);
      const updated = await providersApi.listKeys(provider.id);
      setKeys(Array.isArray(updated) ? updated : []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to remove key');
    }
  };

  const statusIcon = (status: string) => {
    if (status === 'verified') return <CheckCircle className="h-3.5 w-3.5 text-emerald-400" />;
    if (status === 'failed') return <AlertCircle className="h-3.5 w-3.5 text-destructive" />;
    return <Clock className="h-3.5 w-3.5 text-muted-foreground" />;
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-3 border-b border-border px-4 shrink-0 h-[var(--header-height)]">
        <BrandIcon name={provider.icon || provider.id} size={24} />
        <div className="flex-1">
          <h2 className="text-sm font-semibold">{provider.name}</h2>
          <p className="text-2xs text-muted-foreground">{keys.length} key{keys.length !== 1 ? 's' : ''} · {provider.auth_type}</p>
        </div>
        <button onClick={onClose} className="h-8 w-8 flex items-center justify-center rounded-lg hover:bg-accent">
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Tabs */}
      <div className="flex border-b border-border px-5 shrink-0">
        {([
          { id: 'keys' as Tab, label: 'Keys', icon: Shield },
          { id: 'models' as Tab, label: 'Models', icon: Cpu },
          { id: 'usage' as Tab, label: 'Usage', icon: BarChart3 },
        ]).map((t) => {
          const Icon = t.icon;
          const disabled = t.id === 'models' && !hasVerifiedKey;
          return (
            <button key={t.id} onClick={() => !disabled && setTab(t.id)}
              className={cn('flex items-center gap-1.5 px-3 py-2.5 text-xs font-medium border-b-2 transition-colors',
                tab === t.id ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
                disabled && 'opacity-40 cursor-not-allowed')}>
              <Icon className="h-3.5 w-3.5" />{t.label}
            </button>
          );
        })}
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-5">
        {tab === 'keys' && (
          <div className="space-y-3">
            {keys.length === 0 && !showAddKey && (
              <div className="py-8 text-center">
                <Shield className="h-8 w-8 text-muted-foreground/30 mx-auto mb-2" />
                <p className="text-sm text-muted-foreground">No keys configured</p>
                <button onClick={() => setShowAddKey(true)} className="mt-2 text-xs text-primary hover:underline">Add your first key →</button>
              </div>
            )}

            {keys.map((k) => {
              const isRateLimited = k.rate_limited_until && new Date(k.rate_limited_until) > new Date();
              return (
                <div key={k.id} className="rounded-xl border border-border p-3">
                  <div className="flex items-center gap-2 mb-2">
                    {statusIcon(k.status)}
                    <span className="text-xs font-medium flex-1">{k.label || 'Unnamed Key'}</span>
                    <span className="text-2xs font-mono text-muted-foreground">{k.key_hash}</span>
                  </div>
                  <div className="flex items-center gap-3 text-2xs text-muted-foreground">
                    <span>{k.status === 'verified' ? '✓ Verified' : k.status === 'failed' ? '✗ Failed' : '○ Unverified'}</span>
                    {isRateLimited && <span className="text-amber-400">⏳ Rate limited</span>}
                    <span>{k.total_requests?.toLocaleString() || 0} requests</span>
                    {k.last_used_at && <span>Last: {new Date(k.last_used_at).toLocaleTimeString()}</span>}
                  </div>
                  <div className="flex gap-2 mt-2">
                    {k.status !== 'verified' && (
                      <button onClick={() => handleVerify(k.id)} disabled={verifying === k.id}
                        className="rounded-md bg-primary px-2.5 py-1 text-2xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50">
                        {verifying === k.id ? 'Verifying…' : 'Verify'}
                      </button>
                    )}
                    <button onClick={() => handleRetire(k.id)} className="rounded-md border border-border px-2.5 py-1 text-2xs hover:bg-accent">Remove</button>
                  </div>
                </div>
              );
            })}

            {showAddKey ? (
              <div className="rounded-xl border border-primary/30 bg-primary/5 p-3 space-y-2">
                <input value={newLabel} onChange={(e) => setNewLabel(e.target.value)} placeholder="Key label (optional)"
                  className="w-full rounded-lg border border-border bg-background px-3 py-1.5 text-xs" />
                <div className="relative">
                  <input type={showKey ? 'text' : 'password'} value={newKey} onChange={(e) => setNewKey(e.target.value)}
                    placeholder={provider.fields?.[0]?.placeholder || 'Paste API key'}
                    className="w-full rounded-lg border border-border bg-background px-3 py-1.5 text-xs pr-8" />
                  <button onClick={() => setShowKey(!showKey)} className="absolute right-2 top-1.5 text-muted-foreground">
                    {showKey ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                  </button>
                </div>
                <div className="flex gap-2">
                  <button onClick={handleAddKey} disabled={saving || !newKey}
                    className="rounded-lg bg-primary px-3 py-1.5 text-xs text-primary-foreground disabled:opacity-50">
                    {saving ? 'Adding…' : 'Add Key'}
                  </button>
                  <button onClick={() => { setShowAddKey(false); setNewKey(''); }} className="rounded-lg border border-border px-3 py-1.5 text-xs">Cancel</button>
                </div>
              </div>
            ) : keys.length > 0 && (
              <button onClick={() => setShowAddKey(true)}
                className="flex w-full items-center justify-center gap-1.5 rounded-xl border border-dashed border-border py-2.5 text-xs text-muted-foreground hover:border-primary/30 hover:text-foreground">
                <Plus className="h-3.5 w-3.5" /> Add Another Key
              </button>
            )}
          </div>
        )}

        {tab === 'models' && (
          <div>
            {!hasVerifiedKey ? (
              <div className="py-8 text-center">
                <Cpu className="h-8 w-8 text-muted-foreground/30 mx-auto mb-2" />
                <p className="text-sm text-muted-foreground">Verify a key to browse models</p>
              </div>
            ) : modelsError ? (
              <div className="py-8 text-center">
                <AlertCircle className="h-8 w-8 text-destructive/50 mx-auto mb-2" />
                <p className="text-sm text-muted-foreground">Could not load models — check your key is valid</p>
              </div>
            ) : models.length === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">Loading models…</p>
            ) : (
              <div className="space-y-1">
                <p className="text-2xs text-muted-foreground mb-2">Select models to make available for Souls. Star one as default.</p>
                {models.map((m: any) => {
                  const id = typeof m === 'string' ? m : m.id;
                  const name = typeof m === 'string' ? m.split('/').pop() : m.name || m.id;
                  const isSelected = selected.has(id);
                  const isDefault = defaultModel === id;
                  return (
                    <div key={id} className={cn('flex items-center gap-2 rounded-lg border px-3 py-2 transition-colors cursor-pointer',
                      isSelected ? 'border-primary/40 bg-primary/5' : 'border-border hover:border-primary/20')}
                      onClick={() => {
                        if (isSelected) {
                          providersApi.deselectModel(provider.id, id);
                          setSelected((prev) => { const n = new Set(prev); n.delete(id); return n; });
                          if (isDefault) setDefaultModel('');
                        } else {
                          providersApi.selectModel(provider.id, id);
                          setSelected((prev) => new Set(prev).add(id));
                        }
                      }}>
                      <input type="checkbox" checked={isSelected} readOnly className="h-3.5 w-3.5 rounded border-border accent-primary" />
                      <div className="flex-1 min-w-0">
                        <p className="text-xs font-medium truncate">{name}</p>
                      </div>
                      {isSelected && (
                        <button title="Set as default" onClick={(e) => { e.stopPropagation(); providersApi.setDefaultModel(provider.id, id); setDefaultModel(id); }}
                          className={cn('h-5 w-5 flex items-center justify-center rounded', isDefault ? 'text-amber-400' : 'text-muted-foreground/30 hover:text-amber-400')}>
                          ★
                        </button>
                      )}
                      {isDefault && <span className="rounded-full bg-amber-400/10 text-amber-400 px-1.5 py-0.5 text-2xs font-medium shrink-0">Default</span>}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}

        {tab === 'usage' && (
          <div>
            {/* Per-key summary */}
            {keys.filter((k) => k.total_requests > 0).length > 0 && (
              <div className="space-y-2 mb-4">
                <p className="text-xs font-medium text-muted-foreground">Key Usage</p>
                {keys.filter((k) => k.total_requests > 0).map((k) => {
                  const total = keys.reduce((s, x) => s + (x.total_requests || 0), 0);
                  const pct = total > 0 ? (k.total_requests / total) * 100 : 0;
                  return (
                    <div key={k.id} className="flex items-center gap-3">
                      <span className="text-2xs w-24 truncate">{k.label || k.key_hash}</span>
                      <div className="flex-1 h-2 rounded-full bg-muted overflow-hidden">
                        <div className="h-full rounded-full bg-primary" style={{ width: `${pct}%` }} />
                      </div>
                      <span className="text-2xs text-muted-foreground w-16 text-right">{k.total_requests} req</span>
                    </div>
                  );
                })}
              </div>
            )}

            {/* Usage log */}
            <p className="text-xs font-medium text-muted-foreground mb-2">Recent Requests</p>
            {usage.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">No usage yet</p>
            ) : (
              <div className="space-y-1">
                {usage.slice(0, 30).map((log: any) => (
                  <div key={log.id} className="flex items-center gap-2 rounded-lg border border-border px-3 py-1.5 text-2xs">
                    <span className={cn('h-1.5 w-1.5 rounded-full', log.status === 'success' ? 'bg-emerald-400' : 'bg-destructive')} />
                    <span className="text-muted-foreground w-14">{new Date(log.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                    <span className="flex-1 truncate">{log.model}</span>
                    <span className="text-muted-foreground">{log.tokens_in + log.tokens_out} tok</span>
                    <span className="text-muted-foreground">{log.latency_ms}ms</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
