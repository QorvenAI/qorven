'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { Terminal, Globe2, MessageSquare, Database, Clock, Wrench, Mail, Mic } from 'lucide-react';

const CAPABILITIES = [
  { Icon: Terminal,      label: 'Execute code and shell commands on this server' },
  { Icon: Globe2,        label: 'Browse the web, call external APIs, and send HTTP requests' },
  { Icon: MessageSquare, label: 'Send and receive messages via connected channels (Telegram, Slack, WhatsApp, email, and others)' },
  { Icon: Database,      label: 'Read and write files, databases, and connected storage' },
  { Icon: Mail,          label: 'Access and send email from connected accounts' },
  { Icon: Clock,         label: 'Run tasks autonomously on a schedule, without user input' },
  { Icon: Wrench,        label: 'Install packages, run build tools, and invoke system services' },
  { Icon: Mic,           label: 'Process voice input and initiate outbound voice sessions' },
];

export function CapabilitiesNotice({ onConfirmChange, version }: { onConfirmChange: (v: boolean) => void; version: string }) {
  const [confirmed, setConfirmed] = useState(false);
  return (
    <div className="space-y-6">
      <div className="flex items-start gap-4">
        <img src="/logo/qorven-mark.svg" alt="" className="h-11 w-11 mt-0.5 shrink-0" />
        <div>
          <h2 className="text-2xl font-semibold text-foreground leading-tight">Welcome to Qorven</h2>
          {version && (
            <p className="text-sm font-medium text-muted-foreground mt-0.5">Version {version}</p>
          )}
        </div>
      </div>

      <div className="border-t border-border" />

      <p className="text-sm text-secondary-foreground leading-relaxed">
        This wizard will configure Qorven on your system. It takes about 3 minutes.
      </p>

      <div>
        <p className="text-sm text-secondary-foreground leading-relaxed mb-3">
          Qorven is a self-hosted AI agent platform. Once configured, your agents will be able to:
        </p>
        <div className="space-y-2.5">
          {CAPABILITIES.map(({ Icon, label }) => (
            <div key={label} className="flex items-start gap-3">
              <Icon className="h-4 w-4 shrink-0 text-muted-foreground mt-0.5" />
              <span className="text-sm text-foreground leading-snug">{label}</span>
            </div>
          ))}
        </div>
      </div>

      <p className="text-sm text-muted-foreground leading-relaxed">
        All actions are governed by per-agent permissions and approval gates.
        Only install Qorven on systems you own or are authorised to manage.
      </p>

      <label className="flex items-center gap-3 cursor-pointer select-none">
        <input
          type="checkbox"
          checked={confirmed}
          onChange={e => { setConfirmed(e.target.checked); onConfirmChange(e.target.checked); }}
          className="h-4 w-4 rounded border-border accent-primary cursor-pointer"
        />
        <span className="text-sm font-medium text-foreground">I understand and want to proceed</span>
      </label>
    </div>
  );
}
