'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { AlertTriangle, Copy, Check, Plus, Key, Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { userApi } from '@/lib/api';
import { Card, Row, Input, Btn } from './primitives';

interface ApiKey {
  id: string;
  name: string;
  created_at: string;
  last_used_at: string | null;
  preview?: string;
}

export function ApiKeysSettings() {
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [revealed, setRevealed] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const loadKeys = () => {
    userApi.listApiKeys()
      .then((list) => setKeys(list))
      .catch(() => {})
      .finally(() => setLoading(false));
  };

  useEffect(() => { loadKeys(); }, []);

  const create = async () => {
    if (!name.trim()) return;
    setCreating(true);
    try {
      const d = await userApi.createApiKey(name.trim());
      setRevealed(d.key);
      setName('');
      toast.success('API key created — save it now');
      loadKeys();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to create key');
    } finally { setCreating(false); }
  };

  const revoke = async (id: string) => {
    if (!confirm('Revoke this API key? This cannot be undone.')) return;
    setRevoking(id);
    try {
      await userApi.revokeApiKey(id);
      setKeys((prev) => prev.filter((k) => k.id !== id));
      toast.success('Key revoked');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to revoke key');
    } finally { setRevoking(null); }
  };

  const copy = () => {
    if (!revealed) return;
    navigator.clipboard.writeText(revealed);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const fmtDate = (iso: string) => {
    try { return new Date(iso).toLocaleDateString(); } catch { return iso; }
  };

  return (
    <div className="space-y-4">
      {revealed && (
        <div className="rounded-xl border border-amber-400/30 bg-amber-400/5 p-4 space-y-3">
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-4 w-4 text-amber-400 shrink-0" />
            <p className="text-sm font-semibold text-amber-400">Save this key — it will not be shown again</p>
          </div>
          <div className="flex items-center gap-2">
            <code className="flex-1 rounded-lg border border-border bg-background px-3 py-2 text-xs font-mono break-all select-all">
              {revealed}
            </code>
            <button onClick={copy}
              className="h-9 w-9 shrink-0 flex items-center justify-center rounded-lg border border-border hover:bg-accent cursor-pointer transition-colors">
              {copied ? <Check className="h-4 w-4 text-emerald-400" /> : <Copy className="h-4 w-4 text-muted-foreground" />}
            </button>
          </div>
          <button onClick={() => setRevealed(null)} className="text-xs text-muted-foreground hover:text-foreground cursor-pointer">
            I've saved it — dismiss
          </button>
        </div>
      )}

      <Card id="api_keys" title="API Keys"
        description="Programmatic access to the Qorven API. Keys are scoped to your account."
        headerRight={
          <div className="flex items-center gap-2">
            <Input value={name} onChange={setName} placeholder="Key name" className="w-36 text-xs" />
            <Btn onClick={create} loading={creating} disabled={!name.trim()}>
              <Plus className="h-3.5 w-3.5" /> Create
            </Btn>
          </div>
        }>
        {loading ? (
          <div className="flex justify-center py-6">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        ) : keys.length === 0 ? (
          <div className="flex items-center gap-3 rounded-lg border border-dashed border-border px-4 py-5">
            <Key className="h-5 w-5 text-muted-foreground/40 shrink-0" />
            <div>
              <p className="text-sm font-medium">No API keys yet</p>
              <p className="text-xs text-muted-foreground">Enter a name above and click Create to generate your first key.</p>
            </div>
          </div>
        ) : (
          <div className="space-y-2">
            {keys.map((k) => (
              <div key={k.id} className="flex items-center justify-between rounded-xl border border-border px-4 py-3 gap-3">
                <div className="flex items-center gap-3">
                  <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-muted shrink-0">
                    <Key className="h-3.5 w-3.5 text-muted-foreground" />
                  </div>
                  <div>
                    <p className="text-sm font-medium">{k.name}</p>
                    <p className="text-xs text-muted-foreground">
                      Created {fmtDate(k.created_at)}
                      {k.last_used_at && ` · Last used ${fmtDate(k.last_used_at)}`}
                    </p>
                  </div>
                </div>
                <button
                  onClick={() => revoke(k.id)}
                  disabled={revoking === k.id}
                  className="flex items-center gap-1 text-xs text-destructive hover:underline cursor-pointer disabled:opacity-50"
                >
                  {revoking === k.id && <Loader2 className="h-3 w-3 animate-spin" />}
                  Revoke
                </button>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}
