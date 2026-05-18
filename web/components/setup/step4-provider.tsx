'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import { CheckCircle2, RefreshCw, Search, Server, Zap } from 'lucide-react';
import { cn } from '@/lib/utils';
import { BEDROCK_REGIONS, RECOMMENDED_PRIMARY, type ProviderOption, type AddedProvider } from './setup-config';
import { QorvenSpinner, ProviderLogo, ModelPicker, SectionTitle } from './setup-atoms';

export function Step4Provider(p: {
  catalog: ProviderOption[];
  selectedOption: string; setSelectedOption: (v: string) => void;
  region: string; setRegion: (v: string) => void;
  apiKey: string; setApiKey: (v: string) => void;
  apiBase: string; setApiBase: (v: string) => void;
  awsAccessKey: string; setAwsAccessKey: (v: string) => void;
  awsSecretKey: string; setAwsSecretKey: (v: string) => void;
  status: 'idle'|'creating'|'testing'|'ok'|'error'; error: string; sample: string;
  bedrockModels: string[];
  primary: string; setPrimary: (v: string) => void;
  fast: string; setFast: (v: string) => void;
  coding: string; setCoding: (v: string) => void;
  setup: () => void;
  addedProviders: AddedProvider[];
}) {
  const [useIAMRole, setUseIAMRole] = useState(false);
  const [credName, setCredName] = useState('');
  const [dropOpen, setDropOpen] = useState(false);
  const [search, setSearch] = useState('');
  const dropRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  // Close dropdown on outside click
  useEffect(() => {
    function handler(e: MouseEvent) {
      if (dropRef.current && !dropRef.current.contains(e.target as Node)) setDropOpen(false);
    }
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const opt = p.catalog.find(o => o.id === p.selectedOption) ?? p.catalog[0]!;
  const disabled = p.status === 'testing' || p.status === 'creating';
  const isAdded = p.addedProviders.some(a => a.id === p.selectedOption);

  // Auto-fill credential name when provider changes
  useEffect(() => {
    setCredName(opt.label);
  }, [opt.id, opt.label]);

  const canTest = useIAMRole ? true : (
    (!opt.fields.includes('api_key')        || p.apiKey.length > 0) &&
    (!opt.fields.includes('api_base')       || p.apiBase.length > 0 || !!opt.defaultApiBase) &&
    (!opt.fields.includes('aws_access_key') || p.awsAccessKey.length > 0)
  );

  return (
    <div className="space-y-5">
      <SectionTitle icon={Server} title="Connect an LLM provider"
        subtitle="Pick a provider and enter your credentials." />

      {/* Added providers */}
      {p.addedProviders.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {p.addedProviders.map(a => (
            <div key={a.id} className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/10 border border-emerald-500/30 px-2.5 py-0.5 text-xs text-emerald-400">
              <CheckCircle2 className="h-3 w-3" /> {a.name}
            </div>
          ))}
        </div>
      )}

      {/* Provider dropdown */}
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1.5">Provider</label>
        <div className="relative" ref={dropRef}>
          {/* Trigger */}
          <button
            type="button"
            disabled={disabled}
            onClick={() => setDropOpen(o => !o)}
            className={cn(
              'w-full flex items-center gap-2.5 rounded-lg border border-input bg-muted px-3 py-2 text-sm text-foreground transition-colors hover:border-primary/50 disabled:opacity-60',
              dropOpen && 'border-primary'
            )}>
            <ProviderLogo id={opt.id} name={opt.label} size="md" />
            <span className="flex-1 text-left font-medium">{opt.label}</span>
            <svg className={cn('h-4 w-4 text-muted-foreground shrink-0 transition-transform', dropOpen && 'rotate-180')} fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
            </svg>
          </button>

          {/* Menu */}
          {dropOpen && (
            <div className="absolute z-50 mt-1 w-full rounded-xl border border-border bg-popover shadow-lg overflow-hidden">
              {/* Search input */}
              <div className="flex items-center gap-2 px-3 py-2 border-b border-border">
                <Search className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                <input
                  ref={searchRef}
                  autoFocus
                  value={search}
                  onChange={e => setSearch(e.target.value)}
                  placeholder="Search providers…"
                  className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                />
              </div>
              <div className="max-h-56 overflow-y-auto py-1">
                {p.catalog
                  .filter(o => o.label.toLowerCase().includes(search.toLowerCase()))
                  .map(o => {
                    const added = p.addedProviders.some(a => a.id === o.id);
                    const selected = o.id === p.selectedOption;
                    return (
                      <button
                        key={o.id}
                        type="button"
                        onClick={() => { p.setSelectedOption(o.id); setDropOpen(false); setSearch(''); }}
                        className={cn(
                          'w-full flex items-center gap-2.5 px-3 py-2 text-sm transition-colors hover:bg-accent',
                          selected && 'bg-primary/10 text-primary'
                        )}>
                        <ProviderLogo id={o.id} name={o.label} size="md" />
                        <span className="flex-1 text-left font-medium">{o.label}</span>
                        {added && <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400 shrink-0" />}
                        {selected && !added && (
                          <svg className="h-3.5 w-3.5 text-primary shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 13l4 4L19 7" />
                          </svg>
                        )}
                      </button>
                    );
                  })}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Credential name */}
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1.5">Credential name</label>
        <input
          value={credName}
          onChange={e => setCredName(e.target.value)}
          disabled={disabled}
          placeholder={opt.label}
          className="qr-input" />
      </div>

      {/* AWS Bedrock fields */}
      {opt.fields.includes('region') && (
        <>
          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1.5">Region</label>
            <select value={p.region} onChange={e => p.setRegion(e.target.value)} disabled={disabled}
              className="qr-select">
              {BEDROCK_REGIONS.map(r => <option key={r.id} value={r.id}>{r.label}</option>)}
            </select>
          </div>
          <label className="flex items-center gap-2 text-xs text-muted-foreground cursor-pointer">
            <input type="checkbox" checked={useIAMRole} onChange={e => setUseIAMRole(e.target.checked)} className="rounded border-input" />
            Use IAM role / instance profile — no static keys needed
          </label>
          {!useIAMRole && (
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-muted-foreground mb-1.5">Access Key ID</label>
                <input value={p.awsAccessKey} onChange={e => p.setAwsAccessKey(e.target.value)}
                  placeholder="AKIA…" disabled={disabled}
                  className="qr-input" />
              </div>
              <div>
                <label className="block text-xs font-medium text-muted-foreground mb-1.5">Secret Access Key</label>
                <input type="password" value={p.awsSecretKey} onChange={e => p.setAwsSecretKey(e.target.value)}
                  placeholder="••••••••" disabled={disabled}
                  className="qr-input" />
              </div>
            </div>
          )}
        </>
      )}

      {/* API Base */}
      {opt.fields.includes('api_base') && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1.5">
            {opt.id === 'ollama' ? 'Ollama URL' : 'API Base'}
          </label>
          <input value={p.apiBase} onChange={e => p.setApiBase(e.target.value)}
            placeholder={opt.defaultApiBase ?? 'https://api.example.com/v1'} disabled={disabled}
            className="qr-input" />
        </div>
      )}

      {/* API Key */}
      {opt.fields.includes('api_key') && !useIAMRole && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1.5">API Key</label>
          <input type="password" value={p.apiKey} onChange={e => p.setApiKey(e.target.value)}
            placeholder={opt.id === 'openai' ? 'sk-…' : opt.id === 'anthropic' ? 'sk-ant-…' : '••••••••••••'}
            disabled={disabled}
            className="qr-input" />
        </div>
      )}

      {/* Test button */}
      <button onClick={p.setup} disabled={disabled || !canTest}
        className={cn(
          'w-full inline-flex items-center justify-center gap-2 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors disabled:opacity-40 cursor-pointer',
          isAdded && p.status === 'ok'
            ? 'bg-muted border border-border text-foreground hover:bg-accent'
            : 'bg-primary text-primary-foreground hover:bg-primary/90'
        )}>
        {disabled
          ? <><QorvenSpinner className="h-4 w-4" /> Testing…</>
          : p.status === 'ok' && isAdded
          ? <><RefreshCw className="h-4 w-4" /> Re-test connection</>
          : <><Zap className="h-4 w-4" /> Test & Add</>}
      </button>

      {/* Feedback */}
      {p.status === 'error' && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2.5 text-xs text-destructive">
          {p.error}
        </div>
      )}
      {p.status === 'ok' && isAdded && (
        <div className="space-y-3">
          <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-3 py-2.5 text-xs text-emerald-400">
            <div className="flex items-center gap-1.5 font-medium">
              <CheckCircle2 className="h-3.5 w-3.5" /> {opt.label} connected successfully
            </div>
            {p.sample && <div className="mt-1 text-emerald-300/80 italic">"{p.sample}"</div>}
          </div>
          <ModelPicker label="Primary model" value={p.primary} onChange={p.setPrimary}
            options={p.bedrockModels} recommend={RECOMMENDED_PRIMARY} />
          <p className="text-xs text-muted-foreground">
            Fast and coding model routing can be configured in Settings after setup.
          </p>
        </div>
      )}
    </div>
  );
}
