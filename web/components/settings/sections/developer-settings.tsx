'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useRef } from 'react';
import { toast } from 'sonner';
import { Card, Row, Input, Toggle, SaveBar, usePrefs } from './primitives';
import { A2AFederationCard } from './a2a-federation-card';
import { WasmPluginsCard } from './wasm-plugins-card';

export function DeveloperSettings() {
  const { prefs, savePrefs } = usePrefs();
  const [form, setForm] = useState({ debug: false, webhook: '' });
  const [saving, setSaving] = useState(false);
  const loaded = useRef(false);

  useEffect(() => {
    if (loaded.current || !prefs.developer) return;
    loaded.current = true;
    const dev = prefs.developer ?? {};
    setForm({ debug: !!dev.debug, webhook: dev.webhook ?? '' });
  }, [prefs]);

  const save = async () => {
    setSaving(true);
    try {
      await savePrefs({ developer: form });
      toast.success('Developer settings saved');
    } catch { toast.error('Could not save changes. Please try again.'); }
    finally { setSaving(false); }
  };

  return (
    <div className="space-y-4">
      <Card id="developer_tools" title="Developer Tools" description="Debugging and webhook configuration for advanced integrations.">
        <Row label="Debug Mode" hint="Verbose event logging in the Activity panel">
          <Toggle checked={form.debug} onChange={v => setForm(p => ({ ...p, debug: v }))} />
        </Row>
        <Row label="Webhook URL" hint="Receive agent events as HTTP POST to this endpoint">
          <Input value={form.webhook} onChange={v => setForm(p => ({ ...p, webhook: v }))} placeholder="https://your-server.com/webhook" />
        </Row>
        <SaveBar saving={saving} onSave={save} />
      </Card>

      <A2AFederationCard />
      <WasmPluginsCard />
    </div>
  );
}
