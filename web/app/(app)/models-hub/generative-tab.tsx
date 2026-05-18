'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import {
  Cpu, X, Plus, Trash2, Loader2, Check, CheckCircle2, RefreshCw,
  ChevronDown, ChevronUp, ShieldCheck, Key, Zap, BarChart3,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { providers as providersApi } from '@/lib/api';
import { extractErrorMessage } from '@/lib/api-core';
import { toast } from 'sonner';

type ProviderItem = { id: string; name: string; display_name?: string; provider_type: string; api_base?: string; enabled?: boolean };
type ProvKey   = { id: string; label?: string; status: string; usage_count: number; last_used_at?: string };
type LiveModel = { id: string; name?: string };
type SelModel  = { model_id: string; provider_id: string; is_default?: boolean };

function keyPlaceholder(providerType: string, name: string): string {
  const n = name.toLowerCase();
  if (n.includes('gemini') || n.includes('google') || providerType === 'gemini_native') return 'AIza…';
  if (n.includes('anthropic') || providerType === 'anthropic_native') return 'sk-ant-api03-…';
  if (n.includes('groq')) return 'gsk_…';
  if (n.includes('openrouter')) return 'sk-or-…';
  if (n.includes('xai') || n.includes('grok')) return 'xai-…';
  if (n.includes('perplexity')) return 'pplx-…';
  if (n.includes('deepseek')) return 'sk-…';
  if (n.includes('ollama') || n.includes('localhost')) return '(none for local)';
  if (providerType === 'bedrock' || n.includes('bedrock') || n.includes('aws')) return 'AKIA… (AWS access key)';
  if (providerType === 'sagemaker') return 'AKIA… (AWS access key)';
  return 'sk-…';
}

const PRESETS = [
  { name: 'DeepSeek',   type: 'openai_compat',    base: 'https://api.deepseek.com/v1' },
  { name: 'OpenAI',     type: 'openai_compat',    base: 'https://api.openai.com/v1' },
  { name: 'Gemini',     type: 'gemini_native',    base: 'https://generativelanguage.googleapis.com/v1beta' },
  { name: 'Groq',       type: 'openai_compat',    base: 'https://api.groq.com/openai/v1' },
  { name: 'Anthropic',  type: 'anthropic_native', base: 'https://api.anthropic.com' },
  { name: 'Mistral',    type: 'openai_compat',    base: 'https://api.mistral.ai/v1' },
  { name: 'Together',   type: 'openai_compat',    base: 'https://api.together.xyz/v1' },
  { name: 'Fireworks',  type: 'openai_compat',    base: 'https://api.fireworks.ai/inference/v1' },
  { name: 'xAI',        type: 'openai_compat',    base: 'https://api.x.ai/v1' },
  { name: 'Perplexity', type: 'openai_compat',    base: 'https://api.perplexity.ai' },
  { name: 'OpenRouter', type: 'openrouter',       base: 'https://openrouter.ai/api/v1' },
  { name: 'Ollama',     type: 'openai_compat',    base: 'http://localhost:11434/v1' },
];

export function GenerativeTab({ providers, selectedModels, onReload, onSelectionChange }: {
  providers: ProviderItem[];
  selectedModels: SelModel[];
  onReload: () => void;
  onSelectionChange: (m: SelModel[]) => void;
}) {
  const [showAdd, setShowAdd] = useState(false);
  const [form, setForm] = useState({ name: '', type: 'openai_compat', base: '', key: '' });
  const [saving, setSaving] = useState(false);

  const applyPreset = (p: typeof PRESETS[0]) => {
    setForm({ name: p.name.toLowerCase(), type: p.type, base: p.base, key: '' });
  };

  const addProvider = async () => {
    if (!form.name || !form.base) { toast.error('Name and API base are required'); return; }
    setSaving(true);
    try {
      await providersApi.create({ name: form.name, provider_type: form.type, api_base: form.base, api_key: form.key });
      toast.success(`${form.name} added`);
      setShowAdd(false);
      setForm({ name: '', type: 'openai_compat', base: '', key: '' });
      onReload();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed to add provider'); }
    finally { setSaving(false); }
  };

  const deleteProvider = async (p: ProviderItem) => {
    if (!confirm(`Remove "${p.display_name || p.name}"?`)) return;
    try {
      await providersApi.delete(p.id);
      toast.success('Provider removed');
      onReload();
    } catch { toast.error('Failed to remove'); }
  };

  const verifyProvider = async (p: ProviderItem) => {
    try {
      const d: any = await providersApi.verify(p.id);
      if (d?.status === 'ok') toast.success(`${p.display_name || p.name} — verified ✓`);
      else toast.error(extractErrorMessage(d?.error || 'Verification failed'));
    } catch { toast.error('Could not verify provider. Please try again.'); }
  };

  return (
    <div className="space-y-3">
      {!showAdd ? (
        <button onClick={() => setShowAdd(true)}
          className="flex items-center gap-1.5 rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted-foreground hover:text-foreground hover:border-primary/40 hover:bg-accent/30 transition-colors cursor-pointer w-full">
          <Plus className="h-4 w-4" /> Add Provider
        </button>
      ) : (
        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 border-b border-border bg-muted/20">
            <p className="text-sm font-semibold">New Provider</p>
            <button onClick={() => setShowAdd(false)} className="text-muted-foreground hover:text-foreground cursor-pointer">
              <X className="h-4 w-4" />
            </button>
          </div>
          <div className="p-4 space-y-4">
            <div>
              <p className="text-xs text-muted-foreground mb-2">Quick presets</p>
              <div className="flex flex-wrap gap-1.5">
                {PRESETS.map(p => (
                  <button key={p.name} onClick={() => applyPreset(p)}
                    className={cn(
                      'rounded-md border px-2.5 py-1 text-xs transition-colors cursor-pointer',
                      form.name === p.name.toLowerCase() ? 'border-primary bg-primary/10 text-primary' : 'border-border hover:bg-accent',
                    )}>
                    {p.name}
                  </button>
                ))}
              </div>
            </div>
            <div className="grid sm:grid-cols-2 gap-3">
              <div>
                <label className="text-xs text-muted-foreground">Provider Name *</label>
                <input value={form.name} onChange={e => setForm(p => ({ ...p, name: e.target.value }))}
                  placeholder="e.g. deepseek"
                  className="mt-1 qr-input" />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Type</label>
                <select value={form.type} onChange={e => setForm(p => ({ ...p, type: e.target.value }))}
                  className="mt-1 qr-select">
                  <option value="openai_compat">OpenAI-compatible</option>
                  <option value="anthropic_native">Anthropic Native</option>
                  <option value="bedrock">AWS Bedrock</option>
                  <option value="gemini_native">Google Gemini</option>
                </select>
              </div>
              <div className="sm:col-span-2">
                <label className="text-xs text-muted-foreground">API Base URL *</label>
                <input value={form.base} onChange={e => setForm(p => ({ ...p, base: e.target.value }))}
                  placeholder="https://api.example.com/v1"
                  className="mt-1 qr-input" />
              </div>
              <div className="sm:col-span-2">
                <label className="text-xs text-muted-foreground">Initial API Key (optional — add more in Keys tab)</label>
                <input value={form.key} onChange={e => setForm(p => ({ ...p, key: e.target.value }))}
                  placeholder={keyPlaceholder(form.type, form.name)} type="password"
                  className="mt-1 qr-input" />
              </div>
            </div>
            <div className="flex gap-2">
              <button onClick={addProvider} disabled={saving || !form.name || !form.base}
                className="flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
                {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
                Add Provider
              </button>
              <button onClick={() => setShowAdd(false)}
                className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer transition-colors">
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {providers.length === 0 && !showAdd ? (
        <div className="flex flex-col items-center py-16 text-center text-muted-foreground">
          <Cpu className="h-10 w-10 opacity-20 mb-3" />
          <p className="text-sm font-medium">No providers yet</p>
          <p className="text-xs mt-1">Add a provider above to start connecting models.</p>
        </div>
      ) : providers.map(p => (
        <ProviderCard
          key={p.id}
          provider={p}
          selectedModels={selectedModels}
          onVerify={() => verifyProvider(p)}
          onDelete={() => deleteProvider(p)}
          onSelectionChange={onSelectionChange}
        />
      ))}
    </div>
  );
}

function ProviderCard({ provider, selectedModels, onVerify, onDelete, onSelectionChange }: {
  provider: ProviderItem;
  selectedModels: SelModel[];
  onVerify: () => void;
  onDelete: () => void;
  onSelectionChange: (m: SelModel[]) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [keys, setKeys] = useState<ProvKey[]>([]);
  const [keysLoaded, setKeysLoaded] = useState(false);
  const [addingKey, setAddingKey] = useState(false);
  const [keyForm, setKeyForm] = useState({ label: '', key: '' });
  const [savingKey, setSavingKey] = useState(false);
  const [verifyingKey, setVerifyingKey] = useState<string | null>(null);
  const [testingKey, setTestingKey] = useState<string | null>(null);
  const [testResults, setTestResults] = useState<Record<string, { ok: boolean; error?: string; models: { id: string; name: string }[] }>>({});
  const [poolConfig, setPoolConfig] = useState<{ strategy: string; failover_mode: string }>({ strategy: 'priority', failover_mode: 'on_exhaust' });
  const [poolConfigLoaded, setPoolConfigLoaded] = useState(false);
  const [savingPool, setSavingPool] = useState(false);
  const [budgetKey, setBudgetKey] = useState<string | null>(null);
  const [budgetForm, setBudgetForm] = useState<{ usd: string; tokens: string }>({ usd: '', tokens: '' });
  const [discovering, setDiscovering] = useState(false);
  const [liveModels, setLiveModels] = useState<LiveModel[]>([]);
  const [picked, setPicked] = useState<Set<string>>(new Set());

  const provSelected = selectedModels.filter(m => m.provider_id === provider.id);

  const loadKeys = useCallback(async () => {
    try {
      const d = await providersApi.listKeys(provider.id);
      setKeys(Array.isArray(d) ? d : []);
    } catch { setKeys([]); }
    setKeysLoaded(true);
  }, [provider.id]);

  useEffect(() => {
    if (expanded && !keysLoaded) loadKeys();
    if (expanded && !poolConfigLoaded) {
      providersApi.getPoolConfig(provider.id)
        .then(d => { setPoolConfig(d); setPoolConfigLoaded(true); })
        .catch(() => setPoolConfigLoaded(true));
    }
  }, [expanded, keysLoaded, loadKeys, poolConfigLoaded, provider.id]);

  const testKey = async (keyId: string) => {
    setTestingKey(keyId);
    try {
      const d = await providersApi.testKey(keyId);
      setTestResults(prev => ({ ...prev, [keyId]: { ok: d.ok, models: d.models ?? [] } }));
      if (d.ok) {
        toast.success(`Key valid · ${d.models.length} models available`);
        setKeys(prev => prev.map(k => k.id === keyId ? { ...k, status: 'verified' } : k));
        if (liveModels.length === 0 && d.models.length > 0) setLiveModels(d.models);
      } else {
        setTestResults(prev => ({ ...prev, [keyId]: { ok: false, error: extractErrorMessage(d.error || 'Key test failed'), models: [] } }));
      }
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Test failed'); }
    finally { setTestingKey(null); }
  };

  const savePoolConfig = async () => {
    setSavingPool(true);
    try {
      await providersApi.savePoolConfig(provider.id, poolConfig);
      toast.success('Pool strategy saved');
    } catch { toast.error('Could not save changes. Please try again.'); }
    finally { setSavingPool(false); }
  };

  const saveBudget = async (keyId: string) => {
    const usd = budgetForm.usd ? parseFloat(budgetForm.usd) : null;
    const tokens = budgetForm.tokens ? parseInt(budgetForm.tokens) : null;
    try {
      await providersApi.setKeyBudget(keyId, { budget_usd_monthly: usd, budget_tokens_monthly: tokens });
      toast.success('Budget saved');
      setBudgetKey(null);
      loadKeys();
    } catch { toast.error('Could not save budget. Please try again.'); }
  };

  const saveKey = async () => {
    if (!keyForm.key.trim()) return;
    setSavingKey(true);
    try {
      await providersApi.addKey(provider.id, { label: keyForm.label || 'Key', key: keyForm.key });
      toast.success('Key added');
      setKeyForm({ label: '', key: '' });
      setAddingKey(false);
      loadKeys();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed to add key'); }
    finally { setSavingKey(false); }
  };

  const verifyKey = async (keyId: string) => {
    setVerifyingKey(keyId);
    try {
      const d: any = await providersApi.verifyKey(keyId);
      if (d?.status === 'ok' || d?.valid) {
        toast.success('Key is valid ✓');
        setKeys(prev => prev.map(k => k.id === keyId ? { ...k, status: 'verified' } : k));
      } else {
        toast.error(extractErrorMessage(d?.error || 'Key check failed'));
        setKeys(prev => prev.map(k => k.id === keyId ? { ...k, status: 'invalid' } : k));
      }
    } catch { toast.error('Could not verify key. Please try again.'); }
    finally { setVerifyingKey(null); }
  };

  const deleteKey = async (keyId: string) => {
    if (!confirm('Remove this key?')) return;
    try {
      await providersApi.retireKey(keyId);
      toast.success('Key removed');
      setKeys(prev => prev.filter(k => k.id !== keyId));
    } catch { toast.error('Failed to remove key'); }
  };

  const discoverModels = async () => {
    setDiscovering(true);
    try {
      const d: any = await providersApi.liveModels(provider.id);
      const models: LiveModel[] = Array.isArray(d) ? d : (d?.models ?? d?.data ?? []);
      setLiveModels(models);
      const alreadySelected = new Set(provSelected.map(m => m.model_id));
      setPicked(new Set(models.filter(m => alreadySelected.has(m.id)).map(m => m.id)));
      if (models.length === 0) toast.error('No models detected — check provider connectivity');
    } catch { toast.error('Failed to discover models'); }
    finally { setDiscovering(false); }
  };

  const togglePick = (modelId: string) => {
    setPicked(prev => {
      const next = new Set(prev);
      if (next.has(modelId)) next.delete(modelId); else next.add(modelId);
      return next;
    });
  };

  const applySelection = async () => {
    const currentIds = new Set(provSelected.map(m => m.model_id));
    const toAdd = [...picked].filter(id => !currentIds.has(id));
    const toRemove = [...currentIds].filter(id => !picked.has(id));
    try {
      await Promise.all([
        ...toAdd.map(id => providersApi.selectModel(provider.id, id)),
        ...toRemove.map(id => providersApi.deselectModel(provider.id, id)),
      ]);
      const updated = await providersApi.selectedModels();
      onSelectionChange(Array.isArray(updated) ? updated : []);
      toast.success(`Selection updated — ${picked.size} model${picked.size !== 1 ? 's' : ''} active`);
      setLiveModels([]);
    } catch { toast.error('Failed to update model selection'); }
  };

  const verifiedCount = keys.filter(k => k.status === 'verified').length;
  const isRotating = verifiedCount >= 2;

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="flex items-center gap-3 px-4 py-3.5">
        <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-muted shrink-0 text-sm font-semibold">
          {(provider.display_name || provider.name).charAt(0).toUpperCase()}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <p className="text-sm font-medium truncate">{provider.display_name || provider.name}</p>
            {isRotating && (
              <span className="rounded-full bg-emerald-500/10 text-emerald-500 px-2 py-0.5 text-xs font-medium shrink-0">
                Rotating ({verifiedCount} keys)
              </span>
            )}
            {provSelected.length > 0 && (
              <span className="rounded-full bg-primary/10 text-primary px-2 py-0.5 text-xs font-medium shrink-0">
                {provSelected.length} model{provSelected.length !== 1 ? 's' : ''}
              </span>
            )}
          </div>
          <p className="text-xs text-muted-foreground truncate">{provider.provider_type} · {provider.api_base}</p>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium',
            provider.enabled ? 'bg-emerald-500/10 text-emerald-500' : 'bg-muted text-muted-foreground')}>
            {provider.enabled ? 'Active' : 'Disabled'}
          </span>
          <button onClick={onVerify} title="Verify connection"
            className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer transition-colors">
            <ShieldCheck className="h-3.5 w-3.5" />
          </button>
          <button onClick={onDelete} title="Remove provider"
            className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-destructive hover:bg-destructive/10 cursor-pointer transition-colors">
            <Trash2 className="h-3.5 w-3.5" />
          </button>
          <button onClick={() => setExpanded(e => !e)}
            className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer transition-colors">
            {expanded ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          </button>
        </div>
      </div>

      {expanded && (
        <div className="border-t border-border">
          <div className="px-4 py-3 border-b border-border/60">
            <div className="flex items-center justify-between mb-2.5">
              <div className="flex items-center gap-2">
                <Key className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">API Key Pool</span>
                {keys.length > 0 && (
                  <span className="rounded-full bg-muted px-1.5 py-0.5 text-xs">{keys.length}</span>
                )}
              </div>
              <button onClick={() => { setAddingKey(a => !a); setKeyForm({ label: '', key: '' }); }}
                className="flex items-center gap-1 text-xs text-primary hover:underline cursor-pointer">
                <Plus className="h-3 w-3" /> Add Key
              </button>
            </div>

            {addingKey && (
              <div className="rounded-lg border border-border bg-background p-3 mb-2.5 space-y-2">
                <div className="grid grid-cols-2 gap-2">
                  <div>
                    <label className="text-xs text-muted-foreground">Label</label>
                    <input value={keyForm.label} onChange={e => setKeyForm(p => ({ ...p, label: e.target.value }))}
                      placeholder="e.g. Free Tier 1"
                      className="qr-input text-xs" />
                  </div>
                  <div>
                    <label className="text-xs text-muted-foreground">API Key *</label>
                    <input value={keyForm.key} onChange={e => setKeyForm(p => ({ ...p, key: e.target.value }))}
                      placeholder={keyPlaceholder(provider.provider_type, provider.display_name || provider.name)} type="password"
                      className="qr-input text-xs" />
                  </div>
                </div>
                <div className="flex gap-2">
                  <button onClick={saveKey} disabled={savingKey || !keyForm.key.trim()}
                    className="flex items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
                    {savingKey ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />}
                    Save
                  </button>
                  <button onClick={() => setAddingKey(false)}
                    className="rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent cursor-pointer transition-colors">
                    Cancel
                  </button>
                </div>
              </div>
            )}

            {!keysLoaded ? (
              <div className="flex items-center gap-2 py-2 text-xs text-muted-foreground">
                <Loader2 className="h-3 w-3 animate-spin" /> Loading keys…
              </div>
            ) : keys.length === 0 ? (
              <p className="text-xs text-muted-foreground py-1">No keys — add one above to enable this provider.</p>
            ) : (
              <div className="space-y-1.5">
                {keys.map(k => (
                  <div key={k.id}>
                    <div className="flex items-center gap-2 rounded-lg border border-border/60 px-3 py-2">
                      <span className={cn('h-2 w-2 rounded-full shrink-0',
                        k.status === 'verified' ? 'bg-emerald-400' : k.status === 'invalid' ? 'bg-destructive' : 'bg-amber-400')} />
                      <span className="text-xs font-medium truncate flex-1 min-w-0">{k.label || `Key …${k.id.slice(-8)}`}</span>
                      <span className={cn('text-xs px-1.5 py-0.5 rounded shrink-0',
                        k.status === 'verified' ? 'bg-emerald-500/10 text-emerald-500' :
                        k.status === 'invalid'   ? 'bg-destructive/10 text-destructive' :
                        'bg-amber-500/10 text-amber-500')}>
                        {k.status}
                      </span>
                      {(k as any).budget_usd_monthly && (
                        <span className="text-xs text-muted-foreground shrink-0">
                          ${(k as any).spent_usd_month?.toFixed(2)}/${(k as any).budget_usd_monthly}
                        </span>
                      )}
                      <span className="text-xs text-muted-foreground shrink-0 hidden sm:inline">{k.usage_count} req</span>
                      <button onClick={() => testKey(k.id)} disabled={testingKey === k.id} title="Test key + fetch models"
                        className="h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:text-primary hover:bg-primary/10 cursor-pointer transition-colors disabled:opacity-40">
                        {testingKey === k.id ? <Loader2 className="h-3 w-3 animate-spin" /> : <Zap className="h-3 w-3" />}
                      </button>
                      <button onClick={() => { setBudgetKey(budgetKey === k.id ? null : k.id); setBudgetForm({ usd: (k as any).budget_usd_monthly?.toString() ?? '', tokens: (k as any).budget_tokens_monthly?.toString() ?? '' }); }}
                        title="Set budget" className="h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:text-amber-400 hover:bg-accent cursor-pointer transition-colors">
                        <BarChart3 className="h-3 w-3" />
                      </button>
                      <button onClick={() => verifyKey(k.id)} disabled={verifyingKey === k.id} title="Verify key"
                        className="h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer transition-colors disabled:opacity-40">
                        {verifyingKey === k.id ? <Loader2 className="h-3 w-3 animate-spin" /> : <ShieldCheck className="h-3 w-3" />}
                      </button>
                      <button onClick={() => deleteKey(k.id)} title="Remove key"
                        className="h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 cursor-pointer transition-colors">
                        <Trash2 className="h-3 w-3" />
                      </button>
                    </div>
                    {budgetKey === k.id && (
                      <div className="mt-1 rounded-lg border border-amber-400/30 bg-amber-400/5 px-3 py-2.5 space-y-2">
                        <p className="text-xs font-semibold text-amber-400">Monthly Budget</p>
                        <div className="grid grid-cols-2 gap-2">
                          <div>
                            <label className="text-xs text-muted-foreground">USD limit</label>
                            <input value={budgetForm.usd} onChange={e => setBudgetForm(p => ({ ...p, usd: e.target.value }))}
                              placeholder="e.g. 50 (blank = unlimited)" type="number" min="0" step="0.01"
                              className="qr-input text-xs" />
                          </div>
                          <div>
                            <label className="text-xs text-muted-foreground">Token limit</label>
                            <input value={budgetForm.tokens} onChange={e => setBudgetForm(p => ({ ...p, tokens: e.target.value }))}
                              placeholder="e.g. 1000000 (blank = unlimited)" type="number" min="0"
                              className="qr-input text-xs" />
                          </div>
                        </div>
                        <div className="flex gap-2">
                          <button onClick={() => saveBudget(k.id)}
                            className="flex items-center gap-1 rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 cursor-pointer">
                            <Check className="h-3 w-3" /> Save Budget
                          </button>
                          <button onClick={() => setBudgetKey(null)}
                            className="rounded-md border border-border px-3 py-1 text-xs hover:bg-accent cursor-pointer">Cancel</button>
                        </div>
                      </div>
                    )}
                    {testResults[k.id] && !testResults[k.id]!.ok && testResults[k.id]!.error && (
                      <div className="mt-1 rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2">
                        <p className="text-xs text-destructive select-text break-all">{testResults[k.id]!.error}</p>
                      </div>
                    )}
                    {testResults[k.id] && testResults[k.id]!.models.length > 0 && (
                      <div className="mt-1 rounded-lg border border-border/60 bg-muted/20 px-3 py-2">
                        <p className="text-xs text-muted-foreground mb-1.5">{testResults[k.id]!.models.length} models available — select to enable:</p>
                        <div className="flex flex-wrap gap-1">
                          {testResults[k.id]!.models.slice(0, 30).map(m => {
                            const isPicked = picked.has(m.id);
                            return (
                              <button key={m.id} onClick={() => togglePick(m.id)}
                                className={cn('rounded-md border px-2 py-0.5 text-xs transition-colors cursor-pointer',
                                  isPicked ? 'border-primary bg-primary/10 text-primary' : 'border-border hover:bg-accent')}>
                                {m.id}
                              </button>
                            );
                          })}
                        </div>
                        {picked.size > 0 && (
                          <button onClick={applySelection}
                            className="mt-2 flex items-center gap-1 rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 cursor-pointer">
                            <Check className="h-3 w-3" /> Enable {picked.size} model{picked.size !== 1 ? 's' : ''}
                          </button>
                        )}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
            {keys.length >= 2 && (
              <div className="mt-3 pt-3 border-t border-border/40">
                <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">Rotation Strategy</p>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                  {(['priority', 'round_robin', 'random', 'least_used'] as const).map(s => (
                    <button key={s} onClick={() => setPoolConfig(p => ({ ...p, strategy: s }))}
                      className={cn('rounded-lg border px-2.5 py-1.5 text-xs text-left transition-colors cursor-pointer',
                        poolConfig.strategy === s ? 'border-primary bg-primary/10 text-primary font-medium' : 'border-border hover:bg-accent')}>
                      {s === 'priority' ? 'Priority (failover)' : s === 'round_robin' ? 'Round Robin' : s === 'random' ? 'Random' : 'Least Used'}
                    </button>
                  ))}
                </div>
                <div className="mt-2">
                  <p className="text-xs text-muted-foreground mb-1">Rotate on…</p>
                  <div className="flex gap-2">
                    {(['on_exhaust', 'on_error', 'always'] as const).map(f => (
                      <button key={f} onClick={() => setPoolConfig(p => ({ ...p, failover_mode: f }))}
                        className={cn('rounded-md border px-2.5 py-1 text-xs transition-colors cursor-pointer',
                          poolConfig.failover_mode === f ? 'border-primary bg-primary/10 text-primary font-medium' : 'border-border hover:bg-accent')}>
                        {f === 'on_exhaust' ? 'Budget exhaustion' : f === 'on_error' ? 'Error (401/429)' : 'Every request'}
                      </button>
                    ))}
                  </div>
                </div>
                <button onClick={savePoolConfig} disabled={savingPool}
                  className="mt-2 flex items-center gap-1 rounded-md bg-muted hover:bg-accent border border-border px-3 py-1.5 text-xs cursor-pointer transition-colors disabled:opacity-50">
                  {savingPool ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />}
                  Save Strategy
                </button>
              </div>
            )}
          </div>

          <div className="px-4 py-3">
            <div className="flex items-center justify-between mb-2.5">
              <div className="flex items-center gap-2">
                <Zap className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">Models</span>
              </div>
              <button onClick={discoverModels} disabled={discovering}
                className="flex items-center gap-1 text-xs text-primary hover:underline cursor-pointer disabled:opacity-50">
                {discovering ? <Loader2 className="h-3 w-3 animate-spin" /> : <RefreshCw className="h-3 w-3" />}
                {discovering ? 'Discovering…' : 'Discover'}
              </button>
            </div>

            {liveModels.length === 0 && (
              provSelected.length === 0 ? (
                <p className="text-xs text-muted-foreground py-1">Click Discover to fetch available models from this provider.</p>
              ) : (
                <div className="flex flex-wrap gap-1.5">
                  {provSelected.map(m => (
                    <span key={m.model_id} className="flex items-center gap-1 rounded-md border border-border bg-muted/40 px-2 py-0.5 text-xs font-mono">
                      {m.is_default && <span className="h-1.5 w-1.5 rounded-full bg-primary inline-block" />}
                      {m.model_id}
                    </span>
                  ))}
                </div>
              )
            )}

            {liveModels.length > 0 && (
              <div className="space-y-2.5">
                <div className="rounded-lg border border-border overflow-hidden max-h-56 overflow-y-auto">
                  <div className="sticky top-0 flex items-center justify-between px-3 py-2 bg-muted/50 border-b border-border text-xs">
                    <span className="text-muted-foreground">{liveModels.length} models — {picked.size} selected</span>
                    <div className="flex gap-3">
                      <button onClick={() => setPicked(new Set(liveModels.map(m => m.id)))}
                        className="text-primary hover:underline cursor-pointer">All</button>
                      <button onClick={() => setPicked(new Set())}
                        className="text-muted-foreground hover:text-foreground hover:underline cursor-pointer">None</button>
                    </div>
                  </div>
                  {liveModels.map(m => (
                    <label key={m.id}
                      className="flex items-center gap-2.5 px-3 py-2 hover:bg-accent/40 cursor-pointer border-b border-border/40 last:border-0">
                      <div className={cn(
                        'h-4 w-4 rounded border flex items-center justify-center shrink-0 transition-colors',
                        picked.has(m.id) ? 'bg-primary border-primary' : 'border-border bg-background',
                      )}>
                        {picked.has(m.id) && <Check className="h-2.5 w-2.5 text-white" />}
                      </div>
                      <input type="checkbox" className="sr-only" checked={picked.has(m.id)} onChange={() => togglePick(m.id)} />
                      <span className="text-xs font-mono truncate">{m.id}</span>
                      {m.name && m.name !== m.id && <span className="text-xs text-muted-foreground truncate">{m.name}</span>}
                    </label>
                  ))}
                </div>
                <div className="flex gap-2">
                  <button onClick={applySelection}
                    className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 cursor-pointer">
                    <CheckCircle2 className="h-3.5 w-3.5" /> Apply Selection
                  </button>
                  <button onClick={() => setLiveModels([])}
                    className="rounded-lg border border-border px-3 py-1.5 text-xs hover:bg-accent cursor-pointer transition-colors">
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
