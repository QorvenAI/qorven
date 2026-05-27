'use client';

import { useState, useEffect, useCallback } from 'react';
import { ExternalLink, Check, AlertTriangle, Trash2, Plus, Key } from 'lucide-react';
import { toast } from 'sonner';
import { Card, Btn, Input } from './primitives';
import { integrationsApi, RelayKeyRecord } from '@/lib/api-integrations';

const RELAY_PROVIDERS = [
  { id: 'outstand', name: 'Outstand', category: 'social', description: 'Unified social API — handles OAuth, token refresh, rate limits', pricing: '$5/mo (1000 posts)', keyPrefix: 'sk_', keyHint: 'Get key from Outstand dashboard → Settings → API Keys', docsUrl: 'https://www.outstand.so/docs/getting-started' },
  { id: 'postforme', name: 'PostForMe', category: 'social', description: 'Social posting API — white-label OAuth flows', pricing: '$10/mo (1000 posts)', keyPrefix: '', keyHint: 'Get key from app.postforme.dev → API Keys', docsUrl: 'https://api.postforme.dev/docs' },
  { id: 'buffer', name: 'Buffer', category: 'social', description: 'Schedule and publish via Buffer — connect accounts in Buffer dashboard', pricing: 'Free (3 channels) or $5/channel/mo', keyPrefix: '', keyHint: 'Get token from publish.buffer.com → Settings → API', docsUrl: 'https://developers.buffer.com' },
  { id: 'pipedream', name: 'Pipedream', category: 'work', description: 'Work tools — Gmail, Calendar, Slack, Notion, CRM', pricing: 'Free (100 actions/mo)', keyPrefix: 'pd_', keyHint: 'Get key from pipedream.com → Settings → API Keys', docsUrl: 'https://pipedream.com/docs' },
];

function StatusBadge({ status }: { status: string }) {
  const isActive = status === 'active';
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${
      isActive ? 'bg-green-500/10 text-green-600' : 'bg-red-500/10 text-red-600'
    }`}>
      {isActive ? <Check className="h-3 w-3" /> : <AlertTriangle className="h-3 w-3" />}
      {isActive ? 'Active' : 'Error'}
    </span>
  );
}

function ProviderCard({
  provider,
  keys,
  onAdd,
  onDelete,
}: {
  provider: typeof RELAY_PROVIDERS[number];
  keys: RelayKeyRecord[];
  onAdd: (label: string, apiKey: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}) {
  const [showForm, setShowForm] = useState(false);
  const [label, setLabel] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    if (!label.trim() || !apiKey.trim()) return;
    setSaving(true);
    try {
      await onAdd(label.trim(), apiKey.trim());
      setLabel('');
      setApiKey('');
      setShowForm(false);
    } catch (e: any) {
      toast.error(e?.message || 'Failed to save key');
    }
    setSaving(false);
  };

  const handleDelete = async (id: string, keyLabel: string) => {
    if (!window.confirm(`Delete key "${keyLabel}"? This cannot be undone.`)) return;
    await onDelete(id);
  };

  return (
    <Card
      id={`relay-${provider.id}`}
      title={provider.name}
      description={`${provider.description} — ${provider.pricing}`}
      headerRight={
        <div className="flex items-center gap-2">
          <a href={provider.docsUrl} target="_blank" rel="noopener noreferrer"
            className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-primary transition-colors">
            Docs <ExternalLink className="h-3 w-3" />
          </a>
          {!showForm && (
            <Btn variant="ghost" onClick={() => setShowForm(true)}>
              <Plus className="h-3.5 w-3.5" /> Add Key
            </Btn>
          )}
        </div>
      }
    >
      {keys.length > 0 && (
        <div className="space-y-2">
          {keys.map(k => (
            <div key={k.id} className="flex items-center justify-between rounded-lg border border-border px-3 py-2">
              <div className="flex items-center gap-3">
                <Key className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="text-sm font-medium">{k.label}</span>
                <StatusBadge status={k.status} />
                {k.accounts_count > 0 && (
                  <span className="text-xs text-muted-foreground">
                    {k.accounts_count} account{k.accounts_count !== 1 ? 's' : ''}
                  </span>
                )}
              </div>
              <button
                onClick={() => handleDelete(k.id, k.label)}
                className="text-muted-foreground hover:text-destructive transition-colors"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}

      {keys.length === 0 && !showForm && (
        <p className="text-sm text-muted-foreground">No keys configured. Add one to get started.</p>
      )}

      {showForm && (
        <div className="space-y-3 rounded-lg border border-border p-4 bg-muted/30">
          <p className="text-xs text-muted-foreground">{provider.keyHint}</p>
          <div className="flex flex-col sm:flex-row gap-2">
            <Input
              placeholder="Label (e.g. Production)"
              value={label}
              onChange={setLabel}
            />
            <Input
              type="password"
              placeholder={provider.keyPrefix ? `${provider.keyPrefix}...` : 'API key'}
              value={apiKey}
              onChange={setApiKey}
            />
          </div>
          <div className="flex gap-2">
            <Btn variant="primary" loading={saving} disabled={!label.trim() || !apiKey.trim()} onClick={handleSave}>
              Save
            </Btn>
            <Btn variant="ghost" onClick={() => { setShowForm(false); setLabel(''); setApiKey(''); }}>
              Cancel
            </Btn>
          </div>
        </div>
      )}
    </Card>
  );
}

export function IntegrationsSettings() {
  const [keys, setKeys] = useState<RelayKeyRecord[]>([]);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const result = await integrationsApi.listRelayKeys();
      setKeys(result || []);
    } catch {
      /* ignore — endpoint may not exist yet */
    }
    setLoading(false);
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  const handleAdd = async (providerId: string, label: string, apiKey: string) => {
    await integrationsApi.addRelayKey(providerId, label, apiKey);
    toast.success('Key added successfully');
    await refresh();
  };

  const handleDelete = async (id: string) => {
    try {
      await integrationsApi.deleteRelayKeyById(id);
      toast.success('Key removed');
      await refresh();
    } catch {
      toast.error('Failed to delete key');
    }
  };

  if (loading) return <div className="p-6 text-muted-foreground text-sm">Loading...</div>;

  const socialProviders = RELAY_PROVIDERS.filter(p => p.category === 'social');
  const workProviders = RELAY_PROVIDERS.filter(p => p.category === 'work');

  return (
    <div className="space-y-4">
      <div className="space-y-1 mb-2">
        <h2 className="text-base font-semibold text-foreground">Social Relay Providers</h2>
        <p className="text-xs text-muted-foreground">Connect social publishing APIs to post across platforms. Each provider handles OAuth and rate limits for you.</p>
      </div>

      {socialProviders.map(provider => (
        <ProviderCard
          key={provider.id}
          provider={provider}
          keys={keys.filter(k => k.provider === provider.id)}
          onAdd={(label, apiKey) => handleAdd(provider.id, label, apiKey)}
          onDelete={handleDelete}
        />
      ))}

      <div className="space-y-1 mt-6 mb-2">
        <h2 className="text-base font-semibold text-foreground">Work Tools (Pipedream)</h2>
        <p className="text-xs text-muted-foreground">Connect work integrations — email, calendar, CRM, and collaboration tools.</p>
      </div>

      {workProviders.map(provider => (
        <ProviderCard
          key={provider.id}
          provider={provider}
          keys={keys.filter(k => k.provider === provider.id)}
          onAdd={(label, apiKey) => handleAdd(provider.id, label, apiKey)}
          onDelete={handleDelete}
        />
      ))}
    </div>
  );
}
