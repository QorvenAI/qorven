'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { AlertTriangle, Zap, Monitor, Shield, Globe, Bell } from 'lucide-react';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import { Card, Toggle, usePrefs } from './primitives';

const NOTIF_ITEMS = [
  { key: 'notif.qor_alerts',  label: 'Qor Alerts',        desc: 'Errors and attention required by agents',      icon: AlertTriangle },
  { key: 'notif.budget',      label: 'Budget Alerts',      desc: 'Credit usage warnings and limits',             icon: Zap },
  { key: 'notif.cron',        label: 'Cron Jobs',          desc: 'Scheduled task completions',                   icon: Monitor },
  { key: 'notif.approvals',   label: 'Approval Requests',  desc: 'Agents needing your approval to proceed',      icon: Shield },
  { key: 'notif.channels',    label: 'Channel Events',     desc: 'Inbound messages from connected integrations', icon: Globe },
  { key: 'notif.system',      label: 'System Notices',     desc: 'Platform health and maintenance alerts',       icon: Bell },
];

export function NotificationsSettings() {
  const { prefs, savePrefs } = usePrefs();

  const defaults = Object.fromEntries(NOTIF_ITEMS.map(n => [n.key, true]));
  const merged = { ...defaults, ...prefs };

  const toggle = (key: string) => {
    savePrefs({ [key]: !merged[key] }).catch(() => toast.error('Could not save changes. Please try again.'));
  };

  return (
    <div className="space-y-4">
      <Card id="notif_prefs" title="Notification Preferences"
        description="Choose which events trigger in-app notifications for your account.">
        <div className="space-y-2">
          {NOTIF_ITEMS.map(({ key, label, desc, icon: Icon }) => {
            const on = !!merged[key];
            return (
              <div key={key} className="flex items-center justify-between rounded-xl border border-border px-3.5 py-2.5 gap-3">
                <div className="flex items-center gap-3">
                  <div className={cn('flex h-8 w-8 items-center justify-center rounded-lg shrink-0 transition-colors',
                    on ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground')}>
                    <Icon className="h-3.5 w-3.5" />
                  </div>
                  <div>
                    <p className="text-sm font-medium">{label}</p>
                    <p className="text-xs text-muted-foreground">{desc}</p>
                  </div>
                </div>
                <Toggle checked={on} onChange={() => toggle(key)} />
              </div>
            );
          })}
        </div>
      </Card>
    </div>
  );
}
