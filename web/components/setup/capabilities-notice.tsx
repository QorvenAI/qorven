'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { ArrowRight, Terminal, Globe2, MessageSquare, Database, Clock, Wrench } from 'lucide-react';

const CAPABILITIES = [
  { Icon: Terminal,      label: 'Execute code and shell commands' },
  { Icon: Globe2,        label: 'Search the web and call external APIs' },
  { Icon: MessageSquare, label: 'Send messages via connected channels' },
  { Icon: Database,      label: 'Read and write files and databases' },
  { Icon: Clock,         label: 'Run tasks autonomously on a schedule' },
  { Icon: Wrench,        label: 'Install packages and modify system services' },
];

export function CapabilitiesNotice({ onAccept, version }: { onAccept: () => void; version: string }) {
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

      <p className="text-sm font-medium text-secondary-foreground leading-relaxed">
        This wizard will configure Qorven on your system. It takes about 3 minutes.
      </p>

      <div>
        <p className="text-sm font-medium text-secondary-foreground leading-relaxed mb-4">
          Qorven is a self-hosted AI agent platform. Once configured, your agents will be able to:
        </p>
        <div className="space-y-3">
          {CAPABILITIES.map(({ Icon, label }) => (
            <div key={label} className="flex items-center gap-3">
              <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
              <span className="text-sm font-medium text-foreground">{label}</span>
            </div>
          ))}
        </div>
      </div>

      <p className="text-sm text-muted-foreground leading-relaxed">
        All actions are governed by per-agent permissions and approval gates. Only install
        Qorven on systems you own or are authorised to manage.
      </p>

      <label className="flex items-center gap-3 cursor-pointer select-none">
        <input
          type="checkbox"
          checked={confirmed}
          onChange={e => setConfirmed(e.target.checked)}
          className="h-4 w-4 rounded border-border accent-primary cursor-pointer"
        />
        <span className="text-sm font-medium text-foreground">I understand and want to proceed</span>
      </label>

      <div className="flex justify-end">
        <button
          onClick={onAccept}
          disabled={!confirmed}
          className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-2.5 text-sm font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-opacity">
          Begin Setup <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}
