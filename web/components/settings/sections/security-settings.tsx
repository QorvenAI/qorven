'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { toast } from 'sonner';
import { userApi } from '@/lib/api';
import { Card, Row, Input, SaveBar } from './primitives';

export function SecuritySettings() {
  const [form, setForm] = useState({ current: '', next: '', confirm: '' });
  const [saving, setSaving] = useState(false);
  const [showPwd, setShowPwd] = useState(false);

  const f = (key: keyof typeof form) => (v: string) => setForm(p => ({ ...p, [key]: v }));

  const save = async () => {
    if (!form.current) { toast.error('Enter your current password'); return; }
    if (form.next.length < 8) { toast.error('New password must be at least 8 characters'); return; }
    if (form.next !== form.confirm) { toast.error('New passwords do not match'); return; }
    setSaving(true);
    try {
      await userApi.changePassword({ current_password: form.current, new_password: form.next });
      toast.success('Password updated');
      setForm({ current: '', next: '', confirm: '' });
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to update password');
    } finally { setSaving(false); }
  };

  const pwdSuffix = (
    <button type="button" onClick={() => setShowPwd(s => !s)}
      className="text-muted-foreground hover:text-foreground transition-colors cursor-pointer">
      {showPwd ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
    </button>
  );

  return (
    <div className="space-y-4">
      <Card id="change_password" title="Change Password" description="Use a strong password of at least 8 characters.">
        <Row label="Current Password">
          <Input type={showPwd ? 'text' : 'password'} value={form.current} onChange={f('current')} placeholder="••••••••" suffix={pwdSuffix} />
        </Row>
        <Row label="New Password" hint="Min. 8 characters">
          <Input type={showPwd ? 'text' : 'password'} value={form.next} onChange={f('next')} placeholder="••••••••" suffix={pwdSuffix} />
        </Row>
        <Row label="Confirm Password">
          <Input type={showPwd ? 'text' : 'password'} value={form.confirm} onChange={f('confirm')} placeholder="••••••••" suffix={pwdSuffix} />
        </Row>
        <SaveBar saving={saving} onSave={save} label="Update Password" />
      </Card>

      <Card id="session" title="Active Session">
        <div className="flex items-center justify-between rounded-lg border border-border px-4 py-3">
          <div>
            <p className="text-sm font-medium">Current session</p>
            <p className="text-xs text-muted-foreground">Authenticated via JWT token</p>
          </div>
          <span className="flex items-center gap-1.5 rounded-full bg-emerald-500/10 text-emerald-500 px-3 py-1 text-xs font-medium">
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse inline-block" />
            Active
          </span>
        </div>
      </Card>
    </div>
  );
}
