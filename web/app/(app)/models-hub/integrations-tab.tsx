'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import { Loader2, BarChart3, CheckCircle2, Check, Key, ExternalLink } from 'lucide-react';
import { toast } from 'sonner';

const getToken = () =>
  typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

interface Integration {
  id: string;
  name: string;
  desc: string;
  key_url: string;
  configured: boolean;
  key_hint?: string;
}

export function IntegrationsTab() {
  const [integrations, setIntegrations] = useState<Integration[]>([]);
  const [loading, setLoading] = useState(true);
  const [form, setForm] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);

  const load = () => {
    setLoading(true);
    fetch('/api/v1/system/integrations', {
      headers: { Authorization: `Bearer ${getToken()}` },
    })
      .then(r => r.json())
      .then(d => setIntegrations(d?.integrations ?? []))
      .catch(() => {})
      .finally(() => setLoading(false));
  };

  useEffect(() => { load(); }, []);

  const saveKey = async (id: string) => {
    const key = form[id]?.trim();
    if (!key) return;
    setSaving(id);
    try {
      const res = await fetch('/api/v1/system/integrations', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${getToken()}`,
        },
        body: JSON.stringify({ id, api_key: key }),
      });
      if (!res.ok) throw new Error(await res.text());
      setForm(prev => ({ ...prev, [id]: '' }));
      setSaved(id);
      setTimeout(() => setSaved(null), 2000);
      load(); // refresh hint
      const name = integrations.find(i => i.id === id)?.name ?? id;
      toast.success(`${name} API key saved`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to save');
    } finally {
      setSaving(null);
    }
  };

  return (
    <div className="space-y-5">
      <div className="rounded-xl border border-border bg-card overflow-hidden">
        <div className="px-5 py-4 border-b border-border/70 bg-muted/20">
          <h3 className="text-sm font-semibold">Data Integrations</h3>
          <p className="text-xs text-muted-foreground mt-0.5">
            API keys for model intelligence services. Keys are encrypted at rest with the same
            AES-256 encryption used for LLM provider credentials.
          </p>
        </div>
        <div className="px-5 py-4">
          {loading ? (
            <div className="space-y-3">
              {[0, 1].map(i => (
                <div key={i} className="h-20 rounded-lg bg-muted animate-pulse" />
              ))}
            </div>
          ) : (
            <div className="space-y-3">
              {integrations.map(integration => (
                <div key={integration.id} className="rounded-xl border border-border px-4 py-3">
                  <div className="flex items-center justify-between mb-2.5">
                    <div className="flex items-center gap-3">
                      <div className={[
                        'flex h-8 w-8 items-center justify-center rounded-md shrink-0',
                        integration.configured
                          ? 'bg-emerald-500/10 text-emerald-400'
                          : 'bg-muted text-muted-foreground',
                      ].join(' ')}>
                        <BarChart3 className="h-4 w-4" />
                      </div>
                      <div>
                        <div className="flex items-center gap-1.5">
                          <p className="text-sm font-medium">{integration.name}</p>
                          {integration.key_url && (
                            <a
                              href={integration.key_url}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="text-muted-foreground/60 hover:text-primary transition-colors"
                              title={`Get API key at ${integration.key_url}`}
                            >
                              <ExternalLink className="h-3 w-3" />
                            </a>
                          )}
                        </div>
                        <p className="text-2xs text-muted-foreground">{integration.desc}</p>
                        {integration.configured && integration.key_hint && (
                          <p className="text-2xs font-mono text-muted-foreground/70 mt-0.5">
                            {integration.key_hint}
                          </p>
                        )}
                      </div>
                    </div>
                    {integration.configured && (
                      <span className="flex items-center gap-1 text-2xs text-emerald-400 shrink-0">
                        <CheckCircle2 className="h-3 w-3" /> Configured
                      </span>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <input
                      type="password"
                      value={form[integration.id] ?? ''}
                      onChange={e =>
                        setForm(prev => ({ ...prev, [integration.id]: e.target.value }))
                      }
                      placeholder={
                        integration.configured ? 'Enter new key to replace…' : 'Paste API key…'
                      }
                      className="flex-1 qr-input text-xs"
                      onKeyDown={e => e.key === 'Enter' && saveKey(integration.id)}
                    />
                    <button
                      onClick={() => saveKey(integration.id)}
                      disabled={!form[integration.id]?.trim() || saving === integration.id}
                      className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40"
                    >
                      {saving === integration.id ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      ) : saved === integration.id ? (
                        <Check className="h-3.5 w-3.5" />
                      ) : (
                        <Key className="h-3.5 w-3.5" />
                      )}
                      Save
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
