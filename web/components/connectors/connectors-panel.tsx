'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { connectors as connectorsApi } from '@/lib/api';
import { goldConnectors, type ConnectorDef } from './connector-defs';
import { cn } from '@/lib/utils';
import { Check, X, Loader2, Zap, AlertTriangle } from 'lucide-react';

interface ConnectorsPanelProps {
  agentId: string;
  connectedIds?: string[];
}

export function ConnectorsPanel({ agentId, connectedIds = [] }: ConnectorsPanelProps) {
  const [selected, setSelected] = useState<ConnectorDef | null>(null);

  return (
    <div className="space-y-6">
      {/* Grid */}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {goldConnectors.map((c) => {
          const connected = connectedIds.includes(c.id);
          return (
            <button
              key={c.id}
              onClick={() => setSelected(c)}
              className={cn(
                'flex items-center gap-3 rounded-xl border bg-card p-4 text-left transition-colors',
                connected ? 'border-soul-idle/30' : 'border-border hover:border-primary/30',
              )}
            >
              <span className="text-2xl">{c.icon}</span>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">{c.name}</p>
                <p className="text-2xs text-muted-foreground">{c.actions.length} actions · {c.category}</p>
              </div>
              {connected && <Check className="h-4 w-4 text-soul-idle" />}
            </button>
          );
        })}
      </div>

      {/* Auth form slide-over */}
      {selected && (
        <ConnectorAuthForm
          connector={selected}
          agentId={agentId}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}

function ConnectorAuthForm({ connector, agentId, onClose }: { connector: ConnectorDef; agentId: string; onClose: () => void }) {
  const [values, setValues] = useState<Record<string, string>>({});
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<'success' | 'error' | null>(null);

  const handleTest = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      await connectorsApi.test(connector.id, values);
      setTestResult('success');
    } catch {
      setTestResult('error');
    }
    setTesting(false);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-end bg-black/40">
      <div className="h-full w-full max-w-md overflow-y-auto border-s border-border bg-background p-6">
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-2">
            <span className="text-xl">{connector.icon}</span>
            <h2 className="text-lg font-semibold">{connector.name}</h2>
          </div>
          <button onClick={onClose} className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-accent">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="space-y-4">
          {connector.fields.map((f) => (
            <div key={f.key}>
              <label className="text-2sm font-medium">{f.label}{f.required && <span className="text-destructive"> *</span>}</label>
              <input
                type={f.type}
                value={values[f.key] ?? ''}
                onChange={(e) => setValues({ ...values, [f.key]: e.target.value })}
                placeholder={f.placeholder}
                required={f.required}
                className="mt-1 qr-input"
              />
            </div>
          ))}

          {/* Test result */}
          {testResult === 'success' && (
            <div className="flex items-center gap-2 rounded-lg bg-soul-idle/10 px-3 py-2 text-sm text-soul-idle">
              <Check className="h-4 w-4" /> Connection successful
            </div>
          )}
          {testResult === 'error' && (
            <div className="flex items-center gap-2 rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
              <AlertTriangle className="h-4 w-4" /> Connection failed — check credentials
            </div>
          )}

          {/* Actions preview */}
          <div>
            <p className="text-2sm font-medium text-muted-foreground mb-1">Available Actions</p>
            <div className="flex flex-wrap gap-1">
              {connector.actions.map((a) => (
                <span key={a} className="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-2xs">
                  <Zap className="h-2.5 w-2.5" />{a}
                </span>
              ))}
            </div>
          </div>

          <div className="flex gap-2">
            <button
              onClick={handleTest}
              disabled={testing}
              className="flex-1 rounded-lg border border-border px-4 py-2.5 text-sm font-medium hover:bg-accent disabled:opacity-50"
            >
              {testing ? <Loader2 className="mx-auto h-4 w-4 animate-spin" /> : 'Test Connection'}
            </button>
            <button
              onClick={onClose}
              className="flex-1 rounded-lg bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              Save & Connect
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
