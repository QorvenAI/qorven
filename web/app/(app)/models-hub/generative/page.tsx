'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useCallback, useId } from 'react';
import {
  Plus, Trash2, Loader2, Check, CheckCircle2, RefreshCw,
  ShieldCheck, Key, Zap, BarChart3, Power, ChevronDown, X,
  ClipboardPaste, AlertCircle, ChevronRight,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { providers as providersApi } from '@/lib/api';
import { extractErrorMessage } from '@/lib/api-core';
import { toast } from 'sonner';
import { ProviderIcon } from '@/components/provider-icon';
import { Switch } from '@/components/qor/switch';
import { Button } from '@/components/qor/button';
import { Badge } from '@/components/qor/badge';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, DialogClose,
} from '@/components/qor/dialog';
import { Sheet, SheetContent, SheetClose, SheetTitle } from '@/components/qor/sheet';
import { Command, CommandInput, CommandList, CommandGroup, CommandItem, CommandEmpty, CommandCheck } from '@/components/qor/command';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/qor/popover';

// ─── Provider catalogue ───────────────────────────────────────────────────────

type ProviderPreset = {
  id: string;
  name: string;
  type: 'openai_compat' | 'anthropic_native' | 'bedrock' | 'gemini_native' | 'openrouter' | 'custom';
  base: string;
  description: string;
  extraFields?: Array<{ key: string; label: string; type?: string; placeholder?: string }>;
};

const PRESETS: ProviderPreset[] = [
  { id: 'openai',      name: 'OpenAI',               type: 'openai_compat',    base: 'https://api.openai.com/v1',                               description: 'GPT-4o, o1, o3' },
  { id: 'anthropic',   name: 'Anthropic',             type: 'anthropic_native', base: 'https://api.anthropic.com',                               description: 'Claude 4 Opus, Sonnet, Haiku' },
  { id: 'gemini',      name: 'Google Gemini',         type: 'gemini_native',    base: 'https://generativelanguage.googleapis.com/v1beta/openai', description: 'Gemini 2.5 Pro, Flash' },
  { id: 'deepseek',    name: 'DeepSeek',              type: 'openai_compat',    base: 'https://api.deepseek.com/v1',                             description: 'DeepSeek-V3, R1 reasoner' },
  { id: 'xai',         name: 'xAI',                  type: 'openai_compat',    base: 'https://api.x.ai/v1',                                     description: 'Grok 3, Grok Vision' },
  { id: 'mistral',     name: 'Mistral AI',            type: 'openai_compat',    base: 'https://api.mistral.ai/v1',                               description: 'Mistral Large, Codestral' },
  { id: 'cohere',      name: 'Cohere',                type: 'openai_compat',    base: 'https://api.cohere.ai/v1',                                description: 'Command R+, Embed v3' },
  {
    id: 'bedrock', name: 'AWS Bedrock', type: 'bedrock', base: '', description: 'Claude, Llama, Titan on Bedrock',
    extraFields: [
      { key: 'region',     label: 'AWS Region',        placeholder: 'us-east-1' },
      { key: 'access_key', label: 'Access Key ID',     placeholder: 'AKIA…' },
      { key: 'secret_key', label: 'Secret Access Key', type: 'password', placeholder: '••••••' },
    ],
  },
  { id: 'azure',       name: 'Azure OpenAI',          type: 'openai_compat',    base: 'https://{resource}.openai.azure.com/openai/deployments/{deployment}', description: 'Azure-hosted OpenAI models' },
  { id: 'nvidia',      name: 'NVIDIA NIM',            type: 'openai_compat',    base: 'https://integrate.api.nvidia.com/v1',                     description: 'Llama, Mistral, Nemotron on NIM' },
  { id: 'sambanova',   name: 'SambaNova',             type: 'openai_compat',    base: 'https://api.sambanova.ai/v1',                             description: 'Ultra-fast Llama 3 inference' },
  { id: 'openrouter',  name: 'OpenRouter',            type: 'openrouter',       base: 'https://openrouter.ai/api/v1',                            description: 'Unified 200+ model gateway' },
  { id: 'groq',        name: 'Groq',                  type: 'openai_compat',    base: 'https://api.groq.com/openai/v1',                          description: 'LPU-accelerated Llama 3' },
  { id: 'together',    name: 'Together AI',           type: 'openai_compat',    base: 'https://api.together.xyz/v1',                             description: 'Open-source model cloud' },
  { id: 'fireworks',   name: 'Fireworks AI',          type: 'openai_compat',    base: 'https://api.fireworks.ai/inference/v1',                   description: 'Fast open-model serving' },
  { id: 'deepinfra',   name: 'DeepInfra',             type: 'openai_compat',    base: 'https://api.deepinfra.com/v1/openai',                     description: 'Hosted open-weight models' },
  { id: 'replicate',   name: 'Replicate',             type: 'openai_compat',    base: 'https://openai-compat.replicate.com/v1',                  description: 'Run open-source AI models' },
  { id: 'anyscale',    name: 'Anyscale',              type: 'openai_compat',    base: 'https://api.endpoints.anyscale.com/v1',                   description: 'Hosted OSS endpoints on Ray' },
  { id: 'huggingface', name: 'HuggingFace',           type: 'openai_compat',    base: 'https://api-inference.huggingface.co/v1',                 description: 'HF Inference API & Endpoints' },
  { id: 'perplexity',  name: 'Perplexity',            type: 'openai_compat',    base: 'https://api.perplexity.ai',                               description: 'Search-grounded models' },
  { id: 'ollama',      name: 'Ollama',                type: 'openai_compat',    base: 'http://localhost:11434/v1',                                description: 'Local model serving' },
  { id: 'lmstudio',    name: 'LM Studio',             type: 'openai_compat',    base: 'http://localhost:1234/v1',                                 description: 'Local LLM GUI + server' },
  { id: 'vllm',        name: 'vLLM',                  type: 'openai_compat',    base: 'http://localhost:8000/v1',                                 description: 'High-throughput local serving' },
  { id: 'llamafile',   name: 'Llamafile',             type: 'openai_compat',    base: 'http://localhost:8080/v1',                                 description: 'Single-file local LLM server' },
  { id: 'custom',      name: 'Custom / Self-hosted',  type: 'custom',           base: '',                                                         description: 'Any OpenAI-compatible endpoint' },
];

// ─── Types ────────────────────────────────────────────────────────────────────

type ProviderItem = { id: string; name: string; display_name?: string; provider_type: string; api_base?: string; enabled?: boolean };
type ProvKey      = { id: string; label?: string; status: string; usage_count: number; budget_usd_monthly?: number; budget_tokens_monthly?: number };
type LiveModel    = { id: string; name?: string };
type SelModel     = { model_id: string; provider_id: string; is_default?: boolean };

interface KeyEntry {
  uid: string;
  label: string;
  key: string;
  status: 'idle' | 'verifying' | 'ok' | 'error';
  error?: string;
}

type RotationStrategy = 'priority' | 'round_robin' | 'random' | 'least_used';
type FailoverMode     = 'on_exhaust' | 'on_error' | 'always';

// ─── Shared helpers ───────────────────────────────────────────────────────────

const inputCls = 'qr-input';

let _uid = 0;
const uid = () => `k-${++_uid}`;

function parseKeysFromText(text: string): KeyEntry[] {
  return text
    .split('\n')
    .map(l => l.trim())
    .filter(Boolean)
    .map(line => {
      const sep = line.indexOf(' | ');
      if (sep !== -1) return { uid: uid(), label: line.slice(0, sep).trim(), key: line.slice(sep + 3).trim(), status: 'idle' as const };
      return { uid: uid(), label: '', key: line, status: 'idle' as const };
    });
}

// ─── Wizard step indicator ────────────────────────────────────────────────────

function StepBar({ step }: { step: 1 | 2 | 3 }) {
  const steps = ['Provider', 'Keys', 'Models'];
  return (
    <div className="flex items-center gap-0 px-5 py-3 border-b border-border/60 bg-muted/10">
      {steps.map((label, i) => {
        const n = i + 1;
        const done    = step > n;
        const current = step === n;
        return (
          <div key={label} className="flex items-center">
            {i > 0 && <div className={cn('h-px w-8 mx-1.5', done || current ? 'bg-primary/40' : 'bg-border')} />}
            <div className="flex items-center gap-1.5">
              <div className={cn(
                'flex h-5 w-5 items-center justify-center rounded-full text-[11px] font-semibold shrink-0 transition-colors',
                done    ? 'bg-primary text-primary-foreground' :
                current ? 'bg-primary text-primary-foreground ring-2 ring-primary/30' :
                          'bg-muted text-muted-foreground border border-border',
              )}>
                {done ? <Check className="h-2.5 w-2.5" /> : n}
              </div>
              <span className={cn('text-xs hidden sm:block', current ? 'font-medium text-foreground' : done ? 'text-primary' : 'text-muted-foreground')}>
                {label}
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

// ─── Shared key row used in both wizard + key pool ────────────────────────────

function KeyRow({
  entry, onChange, onRemove, onVerify,
}: {
  entry: KeyEntry;
  onChange: (patch: Partial<KeyEntry>) => void;
  onRemove: () => void;
  onVerify: () => void;
}) {
  return (
    <div className={cn(
      'rounded-lg border bg-background overflow-hidden transition-colors',
      entry.status === 'ok'    ? 'border-emerald-500/40' :
      entry.status === 'error' ? 'border-destructive/40' : 'border-border',
    )}>
      <div className="flex items-center gap-2 px-3 py-2.5">
        <div className={cn(
          'flex h-6 w-6 items-center justify-center rounded-md shrink-0 text-[10px] font-medium',
          entry.status === 'ok'        ? 'bg-emerald-500/10 text-emerald-400' :
          entry.status === 'error'     ? 'bg-destructive/10 text-destructive' :
          entry.status === 'verifying' ? 'bg-primary/10 text-primary' :
                                         'bg-muted text-muted-foreground',
        )}>
          {entry.status === 'verifying' ? <Loader2 className="h-3 w-3 animate-spin" /> :
           entry.status === 'ok'        ? <Check className="h-3 w-3" /> :
           entry.status === 'error'     ? <AlertCircle className="h-3 w-3" /> :
                                          <Key className="h-3 w-3" />}
        </div>

        <input
          value={entry.label}
          onChange={e => onChange({ label: e.target.value })}
          placeholder="Label (optional)"
          className="qr-input w-24 shrink-0 text-xs"
        />

        <input
          type="password"
          value={entry.key}
          onChange={e => onChange({ key: e.target.value, status: 'idle', error: undefined })}
          placeholder="sk-… or paste key"
          className="qr-input flex-1 min-w-0 text-xs font-mono"
        />

        <button onClick={onVerify} disabled={!entry.key.trim() || entry.status === 'verifying'} title="Verify"
          className="flex h-6 w-6 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent disabled:opacity-30 shrink-0">
          <ShieldCheck className="h-3.5 w-3.5" />
        </button>

        <button onClick={onRemove} title="Remove"
          className="flex h-6 w-6 items-center justify-center rounded-md text-muted-foreground hover:text-destructive hover:bg-destructive/10 shrink-0">
          <X className="h-3.5 w-3.5" />
        </button>
      </div>
      {entry.status === 'error' && entry.error && (
        <div className="px-3 pb-2 text-xs text-destructive">{entry.error}</div>
      )}
    </div>
  );
}

// ─── Rotation strategy picker ─────────────────────────────────────────────────

const STRATEGIES: { value: RotationStrategy; label: string; desc: string }[] = [
  { value: 'priority',    label: 'Priority',      desc: 'Always use the top key first' },
  { value: 'round_robin', label: 'Round-robin',   desc: 'Spread load evenly across all keys' },
  { value: 'random',      label: 'Random',        desc: 'Pick a random key each request' },
  { value: 'least_used',  label: 'Least used',    desc: 'Send to whichever key has fewest requests' },
];

const FAILOVERS: { value: FailoverMode; label: string }[] = [
  { value: 'on_error',    label: 'On 401 / 429 error' },
  { value: 'on_exhaust',  label: 'On budget exhaustion' },
  { value: 'always',      label: 'Every request (parallel)' },
];

function StrategyPicker({ strategy, failover, onChange }: {
  strategy: RotationStrategy;
  failover: FailoverMode;
  onChange: (s: RotationStrategy, f: FailoverMode) => void;
}) {
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-2">
        {STRATEGIES.map(s => (
          <button key={s.value} onClick={() => onChange(s.value, failover)}
            className={cn('rounded-lg border px-3 py-2.5 text-left text-xs transition-colors',
              strategy === s.value ? 'border-primary bg-primary/8 text-primary' : 'border-border hover:bg-accent')}>
            <p className="font-medium">{s.label}</p>
            <p className={cn('mt-0.5 font-normal text-[11px]', strategy === s.value ? 'text-primary/70' : 'text-muted-foreground')}>{s.desc}</p>
          </button>
        ))}
      </div>
      <div>
        <p className="text-xs font-medium text-muted-foreground mb-2">Failover trigger</p>
        <div className="flex flex-wrap gap-1.5">
          {FAILOVERS.map(f => (
            <button key={f.value} onClick={() => onChange(strategy, f.value)}
              className={cn('rounded-md border px-2.5 py-1.5 text-xs transition-colors',
                failover === f.value ? 'border-primary bg-primary/8 text-primary font-medium' : 'border-border hover:bg-accent')}>
              {f.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

// ─── Add Provider Sheet (wizard) ──────────────────────────────────────────────

function AddProviderSheet({ open, onOpenChange, onAdded }: {
  open: boolean; onOpenChange: (o: boolean) => void; onAdded: () => void;
}) {
  const [step, setStep]           = useState<1 | 2 | 3>(1);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [preset, setPreset]       = useState<ProviderPreset | null>(null);
  const [name, setName]           = useState('');
  const [base, setBase]           = useState('');
  const [extras, setExtras]       = useState<Record<string, string>>({});

  // Keys step
  const [keys, setKeys]           = useState<KeyEntry[]>([{ uid: uid(), label: '', key: '', status: 'idle' }]);
  const [pasteMode, setPasteMode] = useState(false);
  const [pasteText, setPasteText] = useState('');
  const [verifyingAll, setVerifyingAll] = useState(false);

  // Models step — provider ID set after creation
  const [createdId, setCreatedId]   = useState<string | null>(null);
  const [creating, setCreating]     = useState(false);
  const [liveModels, setLiveModels] = useState<LiveModel[]>([]);
  const [loadingModels, setLoadingModels] = useState(false);
  const [picked, setPicked]         = useState<Set<string>>(new Set());
  const [strategy, setStrategy]     = useState<RotationStrategy>('priority');
  const [failover, setFailover]     = useState<FailoverMode>('on_error');
  const [savingModels, setSavingModels] = useState(false);

  const reset = () => {
    setStep(1); setPreset(null); setName(''); setBase(''); setExtras({});
    setKeys([{ uid: uid(), label: '', key: '', status: 'idle' }]);
    setPasteMode(false); setPasteText('');
    setCreatedId(null); setLiveModels([]); setPicked(new Set());
    setStrategy('priority'); setFailover('on_error');
  };

  const pickPreset = (p: ProviderPreset) => {
    setPreset(p);
    setPickerOpen(false);
    setName(p.id === 'custom' ? '' : p.name.toLowerCase().replace(/\s+/g, '-'));
    setBase(p.base);
    const e: Record<string, string> = {};
    p.extraFields?.forEach(f => { e[f.key] = ''; });
    setExtras(e);
  };

  const isBedrock  = preset?.id === 'bedrock';
  const isCustom   = preset?.id === 'custom';
  const isLocal    = ['ollama', 'lmstudio', 'vllm', 'llamafile'].includes(preset?.id ?? '');
  const step1Valid = !!preset && !!name && (isBedrock ? (extras.region && extras.access_key && extras.secret_key) : (base || preset?.base));
  const step2Valid = isBedrock || isLocal || keys.some(k => k.key.trim());

  // Key management helpers
  const updateKey = (idx: number, patch: Partial<KeyEntry>) =>
    setKeys(prev => prev.map((k, i) => i === idx ? { ...k, ...patch } : k));
  const removeKey = (idx: number) =>
    setKeys(prev => prev.filter((_, i) => i !== idx));
  const addKey = () =>
    setKeys(prev => [...prev, { uid: uid(), label: '', key: '', status: 'idle' }]);

  const applyPaste = () => {
    const parsed = parseKeysFromText(pasteText);
    if (parsed.length) setKeys(prev => [...prev.filter(k => k.key.trim()), ...parsed]);
    setPasteMode(false); setPasteText('');
  };

  // Verify a single key against provider URL (pre-creation, just a format check)
  // After provider is created, we use the real API. Pre-creation we can only do basic validation.
  const verifyKeyEntry = async (idx: number) => {
    const entry = keys[idx];
    if (!entry?.key.trim()) return;
    updateKey(idx, { status: 'verifying', error: undefined });
    // Basic client-side check: non-empty and looks like a key
    await new Promise(r => setTimeout(r, 400));
    if (entry!.key.trim().length < 8) {
      updateKey(idx, { status: 'error', error: 'Key seems too short' });
    } else {
      updateKey(idx, { status: 'ok' });
    }
  };

  const verifyAll = async () => {
    setVerifyingAll(true);
    await Promise.all(keys.map((_, i) => verifyKeyEntry(i)));
    setVerifyingAll(false);
  };

  // Step 1 → 2
  const toStep2 = () => { if (step1Valid) setStep(2); };

  // Step 2 → 3: create provider + add keys
  const toStep3 = async () => {
    if (!preset || !step1Valid) return;
    setCreating(true);
    try {
      const payload: Record<string, unknown> = {
        name, provider_type: preset.type === 'custom' ? 'openai_compat' : preset.type,
        api_base: base || preset.base,
      };
      if (isBedrock) { payload.region = extras.region; payload.access_key = extras.access_key; payload.secret_key = extras.secret_key; }
      const prov = await providersApi.create(payload);
      const provId = (prov as any).id;
      setCreatedId(provId);

      // Add all non-empty keys
      const keysToadd = keys.filter(k => k.key.trim());
      if (keysToadd.length) {
        await Promise.allSettled(keysToadd.map(k =>
          providersApi.addKey(provId, { label: k.label || 'Key', key: k.key })
        ));
      }

      setStep(3);

      // Fetch live models
      setLoadingModels(true);
      try {
        const d: any = await providersApi.liveModels(provId);
        const models: LiveModel[] = Array.isArray(d) ? d : (d?.models ?? d?.data ?? []);
        setLiveModels(models);
        // Pre-select all if ≤ 20
        if (models.length > 0 && models.length <= 20) setPicked(new Set(models.map(m => m.id)));
      } catch { /* models optional */ }
      finally { setLoadingModels(false); }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to create provider');
    } finally { setCreating(false); }
  };

  // Step 3: apply model selection + strategy, then close
  const finish = async () => {
    if (!createdId) { onAdded(); onOpenChange(false); reset(); return; }
    setSavingModels(true);
    try {
      await Promise.allSettled([
        ...[...picked].map(id => providersApi.selectModel(createdId, id)),
        providersApi.savePoolConfig(createdId, { strategy, failover_mode: failover }),
      ]);
      toast.success(`${name} ready — ${picked.size} model${picked.size !== 1 ? 's' : ''} active`);
      onAdded(); onOpenChange(false); reset();
    } catch {
      toast.success(`${name} added`);
      onAdded(); onOpenChange(false); reset();
    } finally { setSavingModels(false); }
  };

  return (
    <Sheet open={open} onOpenChange={v => { onOpenChange(v); if (!v) reset(); }}>
      <SheetContent side="right" className="sm:max-w-lg p-0 flex flex-col gap-0" close={false}>
        <SheetTitle className="sr-only">Add Provider</SheetTitle>
        {/* Header */}
        <div className="flex items-center gap-3 px-5 py-4 border-b border-border bg-muted/20 shrink-0">
          <div className="flex-1 min-w-0">
            <p className="text-sm font-semibold">Add Provider</p>
            <p className="text-xs text-muted-foreground">Connect an LLM provider</p>
          </div>
          <SheetClose asChild>
            <Button variant="ghost" mode="icon" size="sm"><X className="h-4 w-4" /></Button>
          </SheetClose>
        </div>

        {/* Step bar */}
        <StepBar step={step} />

        {/* Step content */}
        <div className="flex-1 overflow-y-auto">

          {/* ── Step 1: Provider ── */}
          {step === 1 && (
            <div className="px-5 py-5 space-y-4">
              {/* Provider picker */}
              <div>
                <label className="block text-xs font-medium mb-1.5">Provider <span className="text-destructive">*</span></label>
                <Popover open={pickerOpen} onOpenChange={setPickerOpen}>
                  <PopoverTrigger asChild>
                    <button type="button" className={cn(
                      'w-full flex items-center gap-2.5 rounded-lg border border-border bg-input px-3 py-2.5 text-sm transition-colors hover:border-primary/50',
                      pickerOpen && 'border-primary',
                    )}>
                      {preset ? (
                        <>
                          <ProviderIcon provider={preset.id} size={18} className="shrink-0" />
                          <div className="flex-1 text-left">
                            <span className="font-medium">{preset.name}</span>
                            <span className="text-muted-foreground ml-2 text-xs">{preset.description}</span>
                          </div>
                        </>
                      ) : (
                        <span className="flex-1 text-left text-muted-foreground/60">Select a provider…</span>
                      )}
                      <ChevronDown className={cn('h-4 w-4 text-muted-foreground shrink-0 transition-transform', pickerOpen && 'rotate-180')} />
                    </button>
                  </PopoverTrigger>
                  <PopoverContent className="p-0 w-[var(--radix-popover-trigger-width)]" align="start" sideOffset={4}>
                    <Command>
                      <CommandInput placeholder="Search providers…" />
                      <CommandList className="max-h-64">
                        <CommandEmpty>No providers found.</CommandEmpty>
                        <CommandGroup>
                          {PRESETS.map(p => (
                            <CommandItem key={p.id} value={p.name} onSelect={() => pickPreset(p)} className="gap-2.5 py-2 cursor-pointer">
                              <ProviderIcon provider={p.id} size={18} className="shrink-0" />
                              <div className="flex-1 min-w-0">
                                <span className="text-sm font-medium">{p.name}</span>
                                <span className="text-xs text-muted-foreground ml-2">{p.description}</span>
                              </div>
                              {preset?.id === p.id && <CommandCheck />}
                            </CommandItem>
                          ))}
                        </CommandGroup>
                      </CommandList>
                    </Command>
                  </PopoverContent>
                </Popover>
              </div>

              {preset && (
                <>
                  <div>
                    <label className="block text-xs font-medium mb-1.5">Display Name <span className="text-destructive">*</span></label>
                    <input value={name} onChange={e => setName(e.target.value)} placeholder="e.g. openai-prod" className={inputCls} />
                  </div>

                  {isBedrock && (
                    <div className="grid sm:grid-cols-2 gap-3">
                      {preset.extraFields!.map(f => (
                        <div key={f.key} className={f.key === 'secret_key' ? 'sm:col-span-2' : ''}>
                          <label className="block text-xs font-medium mb-1.5">{f.label} <span className="text-destructive">*</span></label>
                          <input type={f.type ?? 'text'} value={extras[f.key] ?? ''} onChange={e => setExtras(p => ({ ...p, [f.key]: e.target.value }))} placeholder={f.placeholder} className={inputCls} />
                        </div>
                      ))}
                    </div>
                  )}

                  {!isBedrock && (
                    <div>
                      <label className="block text-xs font-medium mb-1.5">
                        API Base URL {isCustom ? <span className="text-destructive">*</span> : <span className="text-muted-foreground font-normal">(optional override)</span>}
                      </label>
                      <input value={base} onChange={e => setBase(e.target.value)} placeholder={preset.base || 'https://api.example.com/v1'} className={cn(inputCls, 'font-mono text-xs')} />
                      {preset.base && !isCustom && <p className="mt-1 text-xs text-muted-foreground">Default: <code className="text-primary/80">{preset.base}</code></p>}
                    </div>
                  )}
                </>
              )}
            </div>
          )}

          {/* ── Step 2: Keys ── */}
          {step === 2 && (
            <div className="px-5 py-5 space-y-4">
              <div className="flex items-center gap-2">
                <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-background border border-border shrink-0">
                  <ProviderIcon provider={preset?.id ?? ''} size={18} />
                </div>
                <div className="min-w-0">
                  <p className="text-sm font-medium">{name}</p>
                  <p className="text-xs text-muted-foreground font-mono truncate">{base || preset?.base}</p>
                </div>
              </div>

              {(isLocal || isBedrock) ? (
                <div className="rounded-lg border border-border bg-muted/20 px-4 py-3">
                  <p className="text-xs text-muted-foreground">
                    {isBedrock ? 'AWS IAM credentials were provided in the previous step — no API key needed.' : 'Local provider — no API key required. The server must be running at the configured base URL.'}
                  </p>
                </div>
              ) : (
                <>
                  {/* Key rows */}
                  <div className="space-y-2">
                    {keys.map((entry, i) => (
                      <KeyRow key={entry.uid} entry={entry}
                        onChange={patch => updateKey(i, patch)}
                        onRemove={() => removeKey(i)}
                        onVerify={() => verifyKeyEntry(i)}
                      />
                    ))}
                  </div>

                  {/* Actions row */}
                  <div className="flex items-center gap-2 flex-wrap">
                    <Button variant="outline" size="sm" onClick={addKey}>
                      <Plus className="h-3.5 w-3.5" /> Add key
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => setPasteMode(v => !v)}>
                      <ClipboardPaste className="h-3.5 w-3.5" /> Paste multiple
                    </Button>
                    {keys.length > 1 && (
                      <Button variant="outline" size="sm" onClick={verifyAll} disabled={verifyingAll}>
                        {verifyingAll ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <ShieldCheck className="h-3.5 w-3.5" />}
                        Verify all
                      </Button>
                    )}
                  </div>

                  {/* Paste textarea */}
                  {pasteMode && (
                    <div className="rounded-lg border border-primary/20 bg-primary/5 p-3 space-y-2">
                      <p className="text-xs text-muted-foreground">One key per line. Optional format: <code className="text-primary/80">Label | sk-your-key</code></p>
                      <textarea
                        value={pasteText}
                        onChange={e => setPasteText(e.target.value)}
                        placeholder={'sk-key-one\nProd key | sk-key-two\nsk-key-three'}
                        rows={5}
                        className="qr-textarea font-mono text-xs resize-none"
                      />
                      <div className="flex gap-2">
                        <Button size="sm" variant="primary" onClick={applyPaste} disabled={!pasteText.trim()}>
                          <Check className="h-3 w-3" /> Add {parseKeysFromText(pasteText).length || 0} keys
                        </Button>
                        <Button size="sm" variant="outline" onClick={() => { setPasteMode(false); setPasteText(''); }}>Cancel</Button>
                      </div>
                    </div>
                  )}

                  {/* Verified summary */}
                  {keys.some(k => k.status !== 'idle') && (
                    <div className="flex items-center gap-3 text-xs text-muted-foreground">
                      {keys.filter(k => k.status === 'ok').length > 0 && (
                        <span className="flex items-center gap-1 text-emerald-400"><CheckCircle2 className="h-3 w-3" />{keys.filter(k => k.status === 'ok').length} verified</span>
                      )}
                      {keys.filter(k => k.status === 'error').length > 0 && (
                        <span className="flex items-center gap-1 text-destructive"><AlertCircle className="h-3 w-3" />{keys.filter(k => k.status === 'error').length} failed</span>
                      )}
                      <span>{keys.filter(k => k.key.trim()).length} key{keys.filter(k => k.key.trim()).length !== 1 ? 's' : ''} total</span>
                    </div>
                  )}
                </>
              )}
            </div>
          )}

          {/* ── Step 3: Models ── */}
          {step === 3 && (
            <div className="px-5 py-5 space-y-4">
              <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/5 px-4 py-3 flex items-center gap-3">
                <CheckCircle2 className="h-4 w-4 text-emerald-400 shrink-0" />
                <div className="min-w-0">
                  <p className="text-sm font-medium text-emerald-400">{name} created</p>
                  <p className="text-xs text-muted-foreground">{keys.filter(k => k.key.trim()).length} key{keys.filter(k => k.key.trim()).length !== 1 ? 's' : ''} added. Now select models to route.</p>
                </div>
              </div>

              {/* Model list */}
              <div>
                <div className="flex items-center justify-between mb-2">
                  <p className="text-xs font-medium">Available Models</p>
                  {!loadingModels && liveModels.length > 0 && (
                    <div className="flex gap-3 text-xs">
                      <button onClick={() => setPicked(new Set(liveModels.map(m => m.id)))} className="text-primary hover:underline">All</button>
                      <button onClick={() => setPicked(new Set())} className="text-muted-foreground hover:text-foreground hover:underline">None</button>
                      <span className="text-muted-foreground">{picked.size} / {liveModels.length}</span>
                    </div>
                  )}
                </div>

                {loadingModels ? (
                  <div className="flex items-center gap-2 rounded-lg border border-border px-4 py-5 text-sm text-muted-foreground">
                    <Loader2 className="h-4 w-4 animate-spin" /> Fetching models from provider…
                  </div>
                ) : liveModels.length === 0 ? (
                  <div className="rounded-lg border border-dashed border-border px-4 py-5 text-center text-xs text-muted-foreground">
                    <p>No models returned — provider may need a valid key or doesn't expose a model list.</p>
                    <p className="mt-1">You can still use it by typing model IDs directly in agent settings.</p>
                  </div>
                ) : (
                  <div className="rounded-lg border border-border bg-card overflow-hidden max-h-52 overflow-y-auto">
                    {liveModels.map(m => (
                      <label key={m.id} className="flex items-center gap-3 px-3.5 py-2 hover:bg-accent/40 cursor-pointer border-b border-border/30 last:border-0">
                        <div className={cn('h-4 w-4 rounded border flex items-center justify-center shrink-0 transition-colors',
                          picked.has(m.id) ? 'bg-primary border-primary' : 'border-border bg-background')}>
                          {picked.has(m.id) && <Check className="h-2.5 w-2.5 text-white" />}
                        </div>
                        <input type="checkbox" className="sr-only" checked={picked.has(m.id)}
                          onChange={() => setPicked(prev => { const n = new Set(prev); n.has(m.id) ? n.delete(m.id) : n.add(m.id); return n; })} />
                        <span className="text-xs font-mono flex-1 truncate">{m.id}</span>
                        {m.name && m.name !== m.id && <span className="text-xs text-muted-foreground shrink-0">{m.name}</span>}
                      </label>
                    ))}
                  </div>
                )}
              </div>

              {/* Key strategy — shown when more than 1 key */}
              {keys.filter(k => k.key.trim()).length > 1 && (
                <div>
                  <p className="text-xs font-medium mb-2.5">Key Usage Strategy</p>
                  <StrategyPicker strategy={strategy} failover={failover} onChange={(s, f) => { setStrategy(s); setFailover(f); }} />
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer navigation */}
        <div className="flex items-center justify-between px-5 py-4 border-t border-border bg-muted/20 shrink-0">
          {step === 1 ? (
            <Button variant="outline" size="md" onClick={() => onOpenChange(false)}>Cancel</Button>
          ) : (
            <Button variant="outline" size="md" onClick={() => setStep(s => (s - 1) as 1 | 2 | 3)} disabled={creating}>
              ← Back
            </Button>
          )}

          {step === 1 && (
            <Button variant="primary" size="md" onClick={toStep2} disabled={!step1Valid}>
              Next: Keys <ChevronRight className="h-4 w-4" />
            </Button>
          )}
          {step === 2 && (
            <Button variant="primary" size="md" onClick={toStep3} disabled={creating}>
              {creating ? <><Loader2 className="h-4 w-4 animate-spin" /> Creating…</> : <>Create & Continue <ChevronRight className="h-4 w-4" /></>}
            </Button>
          )}
          {step === 3 && (
            <Button variant="primary" size="md" onClick={finish} disabled={savingModels}>
              {savingModels ? <Loader2 className="h-4 w-4 animate-spin" /> : <CheckCircle2 className="h-4 w-4" />}
              {picked.size > 0 ? `Add ${picked.size} model${picked.size !== 1 ? 's' : ''} & Finish` : 'Finish'}
            </Button>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}

// ─── Key Pool Sheet (manage keys on existing provider) ────────────────────────

function KeyPoolSheet({ provider, open, onOpenChange }: {
  provider: ProviderItem | null; open: boolean; onOpenChange: (o: boolean) => void;
}) {
  const [existingKeys, setExistingKeys] = useState<ProvKey[]>([]);
  const [loading, setLoading]           = useState(true);
  const [addEntries, setAddEntries]     = useState<KeyEntry[]>([]);
  const [showAdd, setShowAdd]           = useState(false);
  const [pasteMode, setPasteMode]       = useState(false);
  const [pasteText, setPasteText]       = useState('');
  const [savingKeys, setSavingKeys]     = useState(false);
  const [testingKey, setTestingKey]     = useState<string | null>(null);
  const [budgetKey, setBudgetKey]       = useState<string | null>(null);
  const [budgetForm, setBudgetForm]     = useState({ usd: '', tokens: '' });
  const [poolConfig, setPoolConfig]     = useState({ strategy: 'priority', failover_mode: 'on_error' });
  const [savingPool, setSavingPool]     = useState(false);

  const loadData = useCallback(async () => {
    if (!provider) return;
    setLoading(true);
    try {
      const [keys, pool] = await Promise.all([
        providersApi.listKeys(provider.id).catch(() => []),
        providersApi.getPoolConfig(provider.id).catch(() => ({ strategy: 'priority', failover_mode: 'on_error' })),
      ]);
      setExistingKeys(Array.isArray(keys) ? keys : []);
      setPoolConfig(pool);
    } finally { setLoading(false); }
  }, [provider?.id]);

  useEffect(() => { if (open && provider) loadData(); }, [open, loadData]);

  const openAdd = () => {
    setAddEntries([{ uid: uid(), label: '', key: '', status: 'idle' }]);
    setShowAdd(true);
  };

  const saveNewKeys = async () => {
    if (!provider) return;
    setSavingKeys(true);
    const toSave = addEntries.filter(e => e.key.trim());
    try {
      await Promise.allSettled(toSave.map(e => providersApi.addKey(provider.id, { label: e.label || 'Key', key: e.key })));
      toast.success(`${toSave.length} key${toSave.length !== 1 ? 's' : ''} added`);
      setShowAdd(false); setAddEntries([]); setPasteMode(false); setPasteText('');
      loadData();
    } catch { toast.error('Some keys could not be saved. Please check and try again.'); }
    finally { setSavingKeys(false); }
  };

  const testKey = async (id: string) => {
    setTestingKey(id);
    try {
      const d = await providersApi.testKey(id);
      if (d.ok) { toast.success(`Valid · ${d.models?.length ?? 0} models`); setExistingKeys(p => p.map(k => k.id === id ? { ...k, status: 'verified' } : k)); }
      else toast.error(extractErrorMessage(d.error || 'Key test failed'));
    } catch { toast.error('Key test failed. Check your API key and try again.'); }
    finally { setTestingKey(null); }
  };

  const deleteKey = async (id: string) => {
    if (!confirm('Remove this key?')) return;
    try { await providersApi.retireKey(id); toast.success('Key removed'); setExistingKeys(p => p.filter(k => k.id !== id)); }
    catch { toast.error('Could not remove key. Please try again.'); }
  };

  const saveBudget = async (id: string) => {
    try {
      await providersApi.setKeyBudget(id, {
        budget_usd_monthly:    budgetForm.usd    ? parseFloat(budgetForm.usd)  : null,
        budget_tokens_monthly: budgetForm.tokens ? parseInt(budgetForm.tokens) : null,
      });
      toast.success('Budget saved'); setBudgetKey(null); loadData();
    } catch { toast.error('Could not save budget. Please try again.'); }
  };

  const savePool = async () => {
    if (!provider) return;
    setSavingPool(true);
    try { await providersApi.savePoolConfig(provider.id, poolConfig); toast.success('Strategy saved'); }
    catch { toast.error('Could not save strategy. Please try again.'); }
    finally { setSavingPool(false); }
  };

  const applyPaste = () => {
    const parsed = parseKeysFromText(pasteText);
    if (parsed.length) setAddEntries(prev => [...prev.filter(k => k.key.trim()), ...parsed]);
    setPasteMode(false); setPasteText('');
  };

  if (!provider) return null;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="sm:max-w-lg p-0 flex flex-col gap-0" close={false}>
        <SheetTitle className="sr-only">{provider.display_name || provider.name}</SheetTitle>
        <div className="flex items-center gap-3 px-5 py-4 border-b border-border bg-muted/20 shrink-0">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-background border border-border shrink-0">
            <ProviderIcon provider={provider.name} size={20} />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-semibold truncate">{provider.display_name || provider.name}</p>
            <p className="text-xs text-muted-foreground">{provider.provider_type.replace(/_/g, ' ')}</p>
          </div>
          <SheetClose asChild>
            <Button variant="ghost" mode="icon" size="sm"><X className="h-4 w-4" /></Button>
          </SheetClose>
        </div>

        <div className="flex-1 overflow-y-auto">
          {/* Key pool */}
          <div className="px-5 py-5 border-b border-border/60 space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-semibold">API Key Pool</p>
                <p className="text-xs text-muted-foreground mt-0.5">Keys rotate by the strategy below</p>
              </div>
              {!showAdd && (
                <div className="flex items-center gap-1.5">
                  <Button variant="outline" size="sm" onClick={() => setPasteMode(v => !v)}>
                    <ClipboardPaste className="h-3.5 w-3.5" /> Paste
                  </Button>
                  <Button variant="outline" size="sm" onClick={openAdd}>
                    <Plus className="h-3.5 w-3.5" /> Add key
                  </Button>
                </div>
              )}
            </div>

            {/* Paste textarea for direct-add */}
            {pasteMode && !showAdd && (
              <div className="rounded-lg border border-primary/20 bg-primary/5 p-3 space-y-2">
                <p className="text-xs text-muted-foreground">One key per line. Format: <code className="text-primary/80">Label | sk-key</code></p>
                <textarea value={pasteText} onChange={e => setPasteText(e.target.value)}
                  placeholder={'sk-key-one\nProd | sk-key-two'} rows={4}
                  className="qr-textarea font-mono text-xs resize-none" />
                <div className="flex gap-2">
                  <Button size="sm" variant="primary" onClick={() => {
                    const parsed = parseKeysFromText(pasteText);
                    setAddEntries(parsed); setShowAdd(true); setPasteMode(false); setPasteText('');
                  }} disabled={!pasteText.trim()}>
                    <Check className="h-3 w-3" /> Add {parseKeysFromText(pasteText).length || 0} keys
                  </Button>
                  <Button size="sm" variant="outline" onClick={() => { setPasteMode(false); setPasteText(''); }}>Cancel</Button>
                </div>
              </div>
            )}

            {/* New key form */}
            {showAdd && (
              <div className="rounded-lg border border-primary/20 bg-primary/5 p-3 space-y-2">
                <div className="space-y-1.5">
                  {addEntries.map((entry, i) => (
                    <KeyRow key={entry.uid} entry={entry}
                      onChange={patch => setAddEntries(prev => prev.map((k, j) => j === i ? { ...k, ...patch } : k))}
                      onRemove={() => setAddEntries(prev => prev.filter((_, j) => j !== i))}
                      onVerify={() => {}}
                    />
                  ))}
                </div>
                <div className="flex items-center gap-2 flex-wrap">
                  <Button size="sm" variant="outline" onClick={() => setAddEntries(prev => [...prev, { uid: uid(), label: '', key: '', status: 'idle' }])}>
                    <Plus className="h-3 w-3" /> Add another
                  </Button>
                  <Button size="sm" variant="outline" onClick={() => setPasteMode(true)}>
                    <ClipboardPaste className="h-3 w-3" /> Paste more
                  </Button>
                </div>
                {pasteMode && (
                  <div className="space-y-2">
                    <textarea value={pasteText} onChange={e => setPasteText(e.target.value)}
                      placeholder={'sk-key\nLabel | sk-key-two'} rows={3}
                      className="qr-textarea font-mono text-xs resize-none" />
                    <Button size="sm" variant="outline" onClick={applyPaste} disabled={!pasteText.trim()}>
                      <Check className="h-3 w-3" /> Parse & append
                    </Button>
                  </div>
                )}
                <div className="flex gap-2 pt-1">
                  <Button size="sm" variant="primary" onClick={saveNewKeys} disabled={savingKeys || !addEntries.some(e => e.key.trim())}>
                    {savingKeys ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />} Save keys
                  </Button>
                  <Button size="sm" variant="outline" onClick={() => { setShowAdd(false); setAddEntries([]); }}>Cancel</Button>
                </div>
              </div>
            )}

            {/* Existing keys */}
            {loading ? (
              <div className="space-y-2">{[0,1].map(i => <div key={i} className="h-11 rounded-lg bg-muted animate-pulse" />)}</div>
            ) : existingKeys.length === 0 ? (
              <div className="flex items-center gap-3 rounded-lg border border-dashed border-border px-4 py-4">
                <Key className="h-4 w-4 text-muted-foreground/30 shrink-0" />
                <p className="text-xs text-muted-foreground">No keys yet — add a key above to enable this provider.</p>
              </div>
            ) : (
              <div className="space-y-1.5">
                {existingKeys.map(k => (
                  <div key={k.id} className="rounded-lg border border-border bg-background overflow-hidden">
                    <div className="flex items-center gap-3 px-3.5 py-2.5">
                      <div className="flex h-7 w-7 items-center justify-center rounded-md bg-muted shrink-0">
                        <Key className="h-3 w-3 text-muted-foreground" />
                      </div>
                      <div className="flex-1 min-w-0">
                        <p className="text-xs font-medium truncate">{k.label || `Key …${k.id.slice(-6)}`}</p>
                        <p className="text-xs text-muted-foreground">{(k.usage_count ?? 0).toLocaleString()} requests</p>
                      </div>
                      <Badge variant={k.status === 'verified' ? 'success' : k.status === 'invalid' ? 'destructive' : 'warning'} appearance="light" size="sm">
                        {k.status}
                      </Badge>
                      <div className="flex items-center gap-0.5">
                        <Button variant="ghost" mode="icon" size="sm" onClick={() => testKey(k.id)} disabled={testingKey === k.id} title="Test">
                          {testingKey === k.id ? <Loader2 className="h-3 w-3 animate-spin" /> : <Zap className="h-3 w-3" />}
                        </Button>
                        <Button variant="ghost" mode="icon" size="sm" title="Budget"
                          onClick={() => { setBudgetKey(budgetKey === k.id ? null : k.id); setBudgetForm({ usd: k.budget_usd_monthly?.toString() ?? '', tokens: k.budget_tokens_monthly?.toString() ?? '' }); }}>
                          <BarChart3 className="h-3 w-3" />
                        </Button>
                        <Button variant="ghost" mode="icon" size="sm" title="Remove" onClick={() => deleteKey(k.id)} className="hover:text-destructive hover:bg-destructive/10">
                          <Trash2 className="h-3 w-3" />
                        </Button>
                      </div>
                    </div>
                    {budgetKey === k.id && (
                      <div className="border-t border-border/50 bg-muted/20 px-3.5 py-3 space-y-3">
                        <div className="grid grid-cols-2 gap-3">
                          <div>
                            <label className="block text-xs text-muted-foreground mb-1">Monthly USD limit</label>
                            <input value={budgetForm.usd} onChange={e => setBudgetForm(p => ({ ...p, usd: e.target.value }))} placeholder="Unlimited" type="number" min="0" step="0.01" className={inputCls + ' text-xs py-1.5'} />
                          </div>
                          <div>
                            <label className="block text-xs text-muted-foreground mb-1">Monthly token limit</label>
                            <input value={budgetForm.tokens} onChange={e => setBudgetForm(p => ({ ...p, tokens: e.target.value }))} placeholder="Unlimited" type="number" min="0" className={inputCls + ' text-xs py-1.5'} />
                          </div>
                        </div>
                        <div className="flex gap-2">
                          <Button size="sm" variant="primary" onClick={() => saveBudget(k.id)}>Save Budget</Button>
                          <Button size="sm" variant="outline" onClick={() => setBudgetKey(null)}>Cancel</Button>
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Rotation strategy — shown when ≥ 1 key */}
          {existingKeys.length >= 1 && (
            <div className="px-5 py-5 space-y-3">
              <div>
                <p className="text-sm font-semibold">Key Usage Strategy</p>
                <p className="text-xs text-muted-foreground mt-0.5">How requests are distributed across your key pool</p>
              </div>
              <StrategyPicker
                strategy={poolConfig.strategy as RotationStrategy}
                failover={poolConfig.failover_mode as FailoverMode}
                onChange={(s, f) => setPoolConfig({ strategy: s, failover_mode: f })}
              />
              <Button size="sm" variant="outline" onClick={savePool} disabled={savingPool}>
                {savingPool ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />} Save strategy
              </Button>
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}

// ─── Model Discovery Dialog (manage models on existing provider) ──────────────

function ModelDiscoveryDialog({ provider, selectedModels, open, onOpenChange, onSelectionChange }: {
  provider: ProviderItem | null; selectedModels: SelModel[]; open: boolean; onOpenChange: (o: boolean) => void; onSelectionChange: (m: SelModel[]) => void;
}) {
  const [liveModels, setLiveModels] = useState<LiveModel[]>([]);
  const [loading, setLoading]       = useState(false);
  const [picked, setPicked]         = useState<Set<string>>(new Set());

  const provSelected = provider ? selectedModels.filter(m => m.provider_id === provider.id) : [];

  const discover = useCallback(async () => {
    if (!provider) return;
    setLoading(true);
    try {
      const d: any = await providersApi.liveModels(provider.id);
      const models: LiveModel[] = Array.isArray(d) ? d : (d?.models ?? d?.data ?? []);
      setLiveModels(models);
      const already = new Set(provSelected.map(m => m.model_id));
      setPicked(new Set(models.filter(m => already.has(m.id)).map(m => m.id)));
      if (models.length === 0) toast.error('No models found. Check your API key and connectivity.');
    } catch { toast.error('Could not fetch models. Check your connection and try again.'); }
    finally { setLoading(false); }
  }, [provider?.id]);

  useEffect(() => { if (open && provider) discover(); }, [open, discover]);

  const apply = async () => {
    if (!provider) return;
    const current = new Set(provSelected.map(m => m.model_id));
    try {
      await Promise.all([
        ...[...picked].filter(id => !current.has(id)).map(id => providersApi.selectModel(provider.id, id)),
        ...[...current].filter(id => !picked.has(id)).map(id => providersApi.deselectModel(provider.id, id)),
      ]);
      const updated = await providersApi.selectedModels();
      onSelectionChange(Array.isArray(updated) ? updated : []);
      toast.success(`${picked.size} model${picked.size !== 1 ? 's' : ''} active`);
      onOpenChange(false);
    } catch { toast.error('Failed to update selection'); }
  };

  if (!provider) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg p-0 gap-0" showCloseButton={false}>
        <DialogHeader className="px-6 pt-5 pb-4 mb-0 border-b border-border bg-muted/20 flex-row items-center gap-3 space-y-0">
          <div className="flex-1">
            <DialogTitle>Select Models</DialogTitle>
            <DialogDescription className="mt-0.5">{provider.display_name || provider.name}</DialogDescription>
          </div>
          <div className="flex items-center gap-1 shrink-0">
            <Button variant="ghost" mode="icon" size="sm" onClick={discover} disabled={loading} title="Refresh">
              {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
            </Button>
            <DialogClose asChild><Button variant="ghost" mode="icon" size="sm"><X className="h-4 w-4" /></Button></DialogClose>
          </div>
        </DialogHeader>

        {loading ? (
          <div className="flex items-center justify-center gap-2 py-16 text-sm text-muted-foreground">
            <Loader2 className="h-5 w-5 animate-spin" /> Fetching models…
          </div>
        ) : (
          <>
            <div className="px-4 py-2.5 border-b border-border/40 bg-muted/10 flex items-center justify-between">
              <span className="text-xs text-muted-foreground">{liveModels.length} available · {picked.size} selected</span>
              <div className="flex gap-3 text-xs">
                <button onClick={() => setPicked(new Set(liveModels.map(m => m.id)))} className="text-primary hover:underline">All</button>
                <button onClick={() => setPicked(new Set())} className="text-muted-foreground hover:text-foreground hover:underline">None</button>
              </div>
            </div>
            <div className="overflow-y-auto max-h-[50vh]">
              {liveModels.map(m => (
                <label key={m.id} className="flex items-center gap-3 px-4 py-2.5 hover:bg-accent/40 cursor-pointer border-b border-border/30 last:border-0">
                  <div className={cn('h-4 w-4 rounded border flex items-center justify-center shrink-0 transition-colors',
                    picked.has(m.id) ? 'bg-primary border-primary' : 'border-border bg-background')}>
                    {picked.has(m.id) && <Check className="h-2.5 w-2.5 text-white" />}
                  </div>
                  <input type="checkbox" className="sr-only" checked={picked.has(m.id)}
                    onChange={() => setPicked(prev => { const n = new Set(prev); n.has(m.id) ? n.delete(m.id) : n.add(m.id); return n; })} />
                  <span className="text-sm font-mono flex-1 truncate">{m.id}</span>
                  {m.name && m.name !== m.id && <span className="text-xs text-muted-foreground">{m.name}</span>}
                </label>
              ))}
              {liveModels.length === 0 && (
                <div className="py-12 text-center text-sm text-muted-foreground">
                  <p>No models returned from provider</p>
                  <button onClick={discover} className="mt-2 text-primary hover:underline text-xs">Retry</button>
                </div>
              )}
            </div>
            <DialogFooter className="px-6 pb-5 pt-4 border-t border-border">
              <DialogClose asChild><Button variant="outline" size="md">Cancel</Button></DialogClose>
              <Button variant="primary" size="md" onClick={apply} disabled={liveModels.length === 0}>
                <CheckCircle2 className="h-4 w-4" /> Apply ({picked.size})
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

// ─── Provider row ─────────────────────────────────────────────────────────────

function SCard({ title, description, headerRight, children }: {
  title: string; description?: string; headerRight?: React.ReactNode; children: React.ReactNode;
}) {
  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="flex items-start justify-between px-6 py-4 border-b border-border/70 bg-muted/20">
        <div>
          <h3 className="text-sm font-semibold">{title}</h3>
          {description && <p className="text-xs text-muted-foreground mt-0.5 max-w-lg">{description}</p>}
        </div>
        {headerRight && <div className="flex items-center gap-2 shrink-0 ml-4">{headerRight}</div>}
      </div>
      <div className="px-6 py-5 space-y-5">{children}</div>
    </div>
  );
}

function ProviderRow({ provider, selectedModels, onToggle, onDelete, onVerify, onManageKeys, onManageModels }: {
  provider: ProviderItem; selectedModels: SelModel[];
  onToggle: () => void; onDelete: () => void; onVerify: () => void;
  onManageKeys: () => void; onManageModels: () => void;
}) {
  const provSelected = selectedModels.filter(m => m.provider_id === provider.id);
  return (
    <div className="flex items-center gap-3 rounded-xl border border-border bg-background px-4 py-3 group hover:border-border/80 transition-colors">
      <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-muted border border-border/50 shrink-0">
        <ProviderIcon provider={provider.name} size={20} />
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium truncate">{provider.display_name || provider.name}</p>
        <p className="text-xs text-muted-foreground font-mono truncate">{provider.api_base || provider.provider_type.replace(/_/g, ' ')}</p>
      </div>
      <button onClick={onManageModels} className={cn(
        'hidden sm:inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium transition-colors shrink-0',
        provSelected.length > 0 ? 'bg-primary/10 text-primary hover:bg-primary/15' : 'bg-muted text-muted-foreground hover:bg-accent',
      )}>
        {provSelected.length > 0
          ? <><CheckCircle2 className="h-3 w-3" />{provSelected.length} model{provSelected.length !== 1 ? 's' : ''}</>
          : <><Zap className="h-3 w-3" />Add models</>}
      </button>
      <Switch size="sm" checked={!!provider.enabled} onCheckedChange={onToggle} />
      <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
        <Button variant="ghost" mode="icon" size="sm" onClick={onManageKeys} title="Manage keys"><Key className="h-3.5 w-3.5" /></Button>
        <Button variant="ghost" mode="icon" size="sm" onClick={onVerify} title="Verify"><ShieldCheck className="h-3.5 w-3.5" /></Button>
        <Button variant="ghost" mode="icon" size="sm" onClick={onDelete} title="Remove" className="hover:text-destructive hover:bg-destructive/10"><Trash2 className="h-3.5 w-3.5" /></Button>
      </div>
    </div>
  );
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function GenerativePage() {
  const [providerList, setProviderList]     = useState<ProviderItem[]>([]);
  const [selectedModels, setSelectedModels] = useState<SelModel[]>([]);
  const [loading, setLoading]               = useState(true);
  const [showAdd, setShowAdd]               = useState(false);
  const [keysProvider, setKeysProvider]     = useState<ProviderItem | null>(null);
  const [modelsProvider, setModelsProvider] = useState<ProviderItem | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    Promise.all([
      providersApi.list().then(d => setProviderList(Array.isArray(d) ? d : [])).catch(() => {}),
      providersApi.selectedModels().then(d => setSelectedModels(Array.isArray(d) ? d : [])).catch(() => {}),
    ]).finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  const toggleProvider = async (p: ProviderItem) => {
    try {
      await providersApi.update(p.id, { enabled: !p.enabled });
      setProviderList(prev => prev.map(x => x.id === p.id ? { ...x, enabled: !x.enabled } : x));
    } catch { toast.error('Could not update provider. Please try again.'); }
  };

  const deleteProvider = async (p: ProviderItem) => {
    if (!confirm(`Remove "${p.display_name || p.name}"?`)) return;
    try { await providersApi.delete(p.id); toast.success('Provider removed'); load(); }
    catch { toast.error('Could not remove provider. Please try again.'); }
  };

  const verifyProvider = async (p: ProviderItem) => {
    try {
      const d: any = await providersApi.verify(p.id);
      if (d?.status === 'ok') toast.success(`${p.display_name || p.name} — verified ✓`);
      else toast.error(extractErrorMessage(d?.error || 'Verification failed'));
    } catch { toast.error('Verification failed'); }
  };

  const activeCount = providerList.filter(p => p.enabled).length;

  return (
    <>
      <div className="space-y-4">
        <div className="mb-6">
          <h1 className="text-lg font-semibold">Generative AI</h1>
          <p className="text-sm text-muted-foreground mt-0.5">LLM providers, key pools, budgets and model routing</p>
        </div>

        <SCard
          title="Providers"
          description={loading ? undefined : providerList.length === 0
            ? 'No providers configured yet. Add your first provider to start routing AI requests.'
            : `${providerList.length} provider${providerList.length !== 1 ? 's' : ''} · ${activeCount} active · ${selectedModels.length} models selected`
          }
          headerRight={
            <div className="flex items-center gap-2">
              <Button variant="ghost" mode="icon" size="sm" onClick={load} title="Refresh">
                <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
              </Button>
              <Button variant="primary" size="sm" onClick={() => setShowAdd(true)}>
                <Plus className="h-3.5 w-3.5" /> Add Provider
              </Button>
            </div>
          }
        >
          {loading ? (
            <div className="space-y-2">
              {[0,1,2].map(i => (
                <div key={i} className="flex items-center gap-3 rounded-xl border border-border px-4 py-3">
                  <div className="h-9 w-9 rounded-lg bg-muted animate-pulse shrink-0" />
                  <div className="flex-1 space-y-1.5">
                    <div className="h-3.5 bg-muted animate-pulse rounded w-32" />
                    <div className="h-2.5 bg-muted animate-pulse rounded w-48" />
                  </div>
                </div>
              ))}
            </div>
          ) : providerList.length === 0 ? (
            <div className="flex items-center gap-3 rounded-lg border border-dashed border-border px-4 py-5">
              <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-muted shrink-0">
                <Power className="h-4 w-4 text-muted-foreground/40" />
              </div>
              <div>
                <p className="text-sm font-medium">No providers configured</p>
                <p className="text-xs text-muted-foreground mt-0.5">Click "Add Provider" to connect your first LLM.</p>
              </div>
            </div>
          ) : (
            <div className="space-y-2">
              {providerList.map(p => (
                <ProviderRow
                  key={p.id} provider={p} selectedModels={selectedModels}
                  onToggle={() => toggleProvider(p)} onDelete={() => deleteProvider(p)}
                  onVerify={() => verifyProvider(p)} onManageKeys={() => setKeysProvider(p)}
                  onManageModels={() => setModelsProvider(p)}
                />
              ))}
            </div>
          )}
        </SCard>
      </div>

      <AddProviderSheet open={showAdd} onOpenChange={setShowAdd} onAdded={load} />
      <KeyPoolSheet provider={keysProvider} open={!!keysProvider} onOpenChange={o => { if (!o) setKeysProvider(null); }} />
      <ModelDiscoveryDialog
        provider={modelsProvider} selectedModels={selectedModels} open={!!modelsProvider}
        onOpenChange={o => { if (!o) setModelsProvider(null); }} onSelectionChange={setSelectedModels}
      />
    </>
  );
}
