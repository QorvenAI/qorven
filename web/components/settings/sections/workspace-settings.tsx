'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useRef } from 'react';
import { toast } from 'sonner';
import { Card, Row, Input, SaveBar, TimezoneSelect, usePrefs } from './primitives';

export function WorkspaceSettings() {
  const { prefs, savePrefs } = usePrefs();
  const [form, setForm] = useState({ platform_name: '', default_model: '', timezone: '', budget_reset_tz: '' });
  const [saving, setSaving] = useState(false);
  const loaded = useRef(false);

  const browserTZ = typeof Intl !== 'undefined' ? Intl.DateTimeFormat().resolvedOptions().timeZone : 'UTC';

  useEffect(() => {
    if (loaded.current) return;
    const ws = prefs.workspace ?? {};
    if (Object.keys(prefs).length === 0) return;
    loaded.current = true;
    setForm({
      platform_name:   ws.platform_name  ?? '',
      default_model:   ws.default_model  ?? '',
      timezone:        ws.timezone        ?? '',
      budget_reset_tz: ws.budget_reset_tz ?? '',
    });
  }, [prefs]);

  const save = async () => {
    setSaving(true);
    try {
      await savePrefs({ workspace: form });
      toast.success('Workspace settings saved');
    } catch { toast.error('Could not save changes. Please try again.'); }
    finally { setSaving(false); }
  };

  return (
    <div className="space-y-4">
      <Card id="workspace" title="Workspace" description="Tenant-level configuration for your Qorven instance.">
        <Row label="Platform Name" hint="Shown in the browser tab and outbound emails">
          <Input value={form.platform_name} onChange={v => setForm(p => ({ ...p, platform_name: v }))} placeholder="Qorven" />
        </Row>
        <Row label="Default Model" hint="Fallback model ID for newly created agents">
          <Input value={form.default_model} onChange={v => setForm(p => ({ ...p, default_model: v }))} placeholder="e.g. kimi-k2.5" />
        </Row>
        <Row label="Timezone" hint={`Used for schedules and budget reset dates. Browser: ${browserTZ}`}>
          <TimezoneSelect value={form.timezone || browserTZ} onChange={v => setForm(p => ({ ...p, timezone: v }))} />
        </Row>
        <Row label="Budget Reset Timezone" hint="Override for monthly budget resets only. Leave blank to use Timezone above.">
          <TimezoneSelect value={form.budget_reset_tz || form.timezone || browserTZ} onChange={v => setForm(p => ({ ...p, budget_reset_tz: v }))} />
        </Row>
        <SaveBar saving={saving} onSave={save} />
      </Card>
    </div>
  );
}
