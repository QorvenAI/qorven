'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { channels as channelsApi } from '@/lib/api';
import { channelFormSchemas, type ChannelField } from './channel-schemas';
import { cn } from '@/lib/utils';
import { X, Loader2 } from 'lucide-react';
import { BrandIcon } from '@/components/brand-icon';
import type { Channel, ChannelType } from '@/types';
import { WhatsAppQRPanel } from './whatsapp-qr-panel';
import { SetupGuide } from './channel-setup-guide';

interface ChannelConnectFormProps {
  agentId: string;
  channelType: ChannelType;
  /** Pass an existing channel to enter edit mode */
  existing?: Channel;
  onClose: () => void;
  onConnected: () => void;
}

export function ChannelConnectForm({ agentId, channelType, existing, onClose, onConnected }: ChannelConnectFormProps) {
  const schema = channelFormSchemas[channelType];

  // Seed values from existing config in edit mode
  const [values, setValues] = useState<Record<string, string>>(() => {
    if (!existing?.config) return {};
    return Object.fromEntries(
      Object.entries(existing.config).map(([k, v]) => [k, String(v ?? '')])
    );
  });
  const [name, setName] = useState<string>(existing?.name ?? '');
  const [savedChannelId, setSavedChannelId] = useState<string | null>(existing?.id ?? null);
  const [showQR, setShowQR] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [testState, setTestState] = useState<'idle' | 'testing' | 'ok' | 'fail'>('idle');
  const [testMessage, setTestMessage] = useState<string>('');

  const set = (key: string, val: string) => setValues((prev) => ({ ...prev, [key]: val }));

  const handleTest = async () => {
    if (!savedChannelId) return;
    setTestState('testing');
    setTestMessage('');
    try {
      const result = await channelsApi.test(savedChannelId);
      if (result.ok) {
        setTestState('ok');
        setTestMessage(result.message ?? 'Connection successful');
      } else {
        setTestState('fail');
        setTestMessage(result.error ?? 'Connection failed');
      }
    } catch (err) {
      setTestState('fail');
      setTestMessage(err instanceof Error ? err.message : 'Test failed');
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    const channelName = name.trim() || schema.label;
    try {
      if (existing) {
        await channelsApi.update(existing.id, { name: channelName, config: values });
      } else {
        const created = await channelsApi.create({ agent_id: agentId, channel_type: channelType, name: channelName, config: values, enabled: true });
        setSavedChannelId(created.id);
      }
      if (channelType === 'whatsapp' && (values as Record<string, string>).mode === 'bridge') {
        setShowQR(true);
      } else {
        onConnected();
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save channel');
    } finally {
      setSubmitting(false);
    }
  };

  // Only show fields whose showWhen condition is met (or unconditional fields)
  const visibleFields = schema.fields.filter((f) => {
    if (!f.showWhen) return true;
    return values[f.showWhen.key] === f.showWhen.value;
  });

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-end bg-black/40" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="h-full w-full max-w-md overflow-y-auto border-s border-border bg-background">
        {/* Header */}
        <div className="sticky top-0 z-10 flex items-center gap-3 border-b border-border bg-background px-5 py-4">
          <span className={cn('flex h-9 w-9 items-center justify-center rounded-xl', schema.color)}>
            <BrandIcon name={channelType} size={20} useBrandColor={false} />
          </span>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-semibold">{existing ? 'Edit' : 'Connect'} {schema.label}</p>
            <p className="text-2xs text-muted-foreground truncate">{schema.description}</p>
          </div>
          <button onClick={onClose} className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-accent">
            <X className="h-4 w-4" />
          </button>
        </div>

        {showQR && savedChannelId ? (
          <div className="space-y-4 p-5">
            <WhatsAppQRPanel channelId={savedChannelId} />
            <button
              className="w-full rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground"
              onClick={onConnected}
            >
              Done
            </button>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4 p-5">
            {/* Name — visible display label for this channel instance */}
            <div>
              <label className="text-2sm font-medium">Channel Name</label>
              <input
                type="text"
                value={name}
                onChange={e => setName(e.target.value)}
                placeholder={`e.g. Prime's ${schema.label}`}
                className="mt-1 qr-input"
              />
              <p className="mt-1 text-2xs text-muted-foreground">
                How this channel appears in the UI — e.g. &quot;Prime&apos;s Telegram&quot; or &quot;Support Bot&quot;
              </p>
            </div>

            {visibleFields.map((field) => (
              <FieldInput key={field.key} field={field} value={values[field.key] ?? ''} onChange={(v) => set(field.key, v)} />
            ))}

            {error && <p className="rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}

            <SetupGuide schema={schema} />

            <button
              type="submit"
              disabled={submitting}
              className="w-full rounded-lg bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {submitting
                ? <Loader2 className="mx-auto h-4 w-4 animate-spin" />
                : existing ? `Save Changes` : `Connect ${schema.label}`}
            </button>

            {channelType === 'email' && savedChannelId && (
              <div className="space-y-2">
                <button
                  type="button"
                  onClick={handleTest}
                  disabled={testState === 'testing'}
                  className="w-full rounded-lg border border-border px-4 py-2.5 text-sm font-medium text-foreground hover:bg-accent disabled:opacity-50"
                >
                  {testState === 'testing'
                    ? <Loader2 className="mx-auto h-4 w-4 animate-spin" />
                    : 'Test IMAP Connection'}
                </button>
                {testState === 'ok' && (
                  <p className="rounded-lg bg-emerald-500/10 px-3 py-2 text-sm text-emerald-400">{testMessage}</p>
                )}
                {testState === 'fail' && (
                  <p className="rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">{testMessage}</p>
                )}
              </div>
            )}
          </form>
        )}
      </div>
    </div>
  );
}

function FieldInput({ field, value, onChange }: { field: ChannelField; value: string; onChange: (v: string) => void }) {
  if (field.type === 'toggle') {
    return (
      <label className="flex items-center justify-between py-1">
        <span className="text-sm">{field.label}</span>
        <button
          type="button"
          onClick={() => onChange(value === 'true' ? 'false' : 'true')}
          className={cn('relative h-5 w-9 rounded-full transition-colors', value === 'true' ? 'bg-primary' : 'bg-muted')}
        >
          <span className={cn('absolute top-0.5 h-4 w-4 rounded-full bg-background shadow transition-transform', value === 'true' ? 'translate-x-4' : 'translate-x-0.5')} />
        </button>
      </label>
    );
  }

  if (field.type === 'select') {
    return (
      <div>
        <label className="text-2sm font-medium">
          {field.label}{field.required && <span className="text-destructive"> *</span>}
        </label>
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          required={field.required}
          className="mt-1 qr-select"
        >
          {field.options?.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
        {field.help && <p className="mt-1 text-2xs text-muted-foreground">{field.help}</p>}
      </div>
    );
  }

  if (field.type === 'textarea') {
    return (
      <div>
        <label className="text-2sm font-medium">
          {field.label}{field.required && <span className="text-destructive"> *</span>}
        </label>
        <textarea
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder}
          required={field.required}
          rows={5}
          className="mt-1 qr-textarea font-mono text-xs"
        />
        {field.help && <p className="mt-1 text-2xs text-muted-foreground">{field.help}</p>}
      </div>
    );
  }

  return (
    <div>
      <label className="text-2sm font-medium">
        {field.label}{field.required && <span className="text-destructive"> *</span>}
      </label>
      <input
        type={field.type === 'password' ? 'password' : 'text'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={field.placeholder}
        required={field.required}
        className="mt-1 qr-input"
      />
      {field.help && <p className="mt-1 text-2xs text-muted-foreground">{field.help}</p>}
    </div>
  );
}
