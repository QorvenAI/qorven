'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import {
  ArrowRight, Users, Brain, Workflow, GitBranch,
  MessageSquare, Code2, Mail, Globe2, Mic,
} from 'lucide-react';

const CAPABILITIES = [
  { Icon: Users,         label: '42 specialised agents',         desc: 'CEO → Prime → C-suite → workers. Governed by org hierarchy and approval gates.' },
  { Icon: Brain,         label: 'Any AI model',                  desc: 'Claude, GPT, Gemini, DeepSeek, local models — switch per agent, no lock-in.' },
  { Icon: Globe2,        label: '20+ channels',                  desc: 'Telegram, WhatsApp, Slack, Discord, email, Teams, and more. Agents live where your team does.' },
  { Icon: Workflow,      label: 'Autonomous workflows',          desc: 'Visual drag-to-connect editor. Agents run tasks on a schedule or trigger from events.' },
  { Icon: Code2,         label: 'Full-stack code IDE',           desc: 'Write, test, and deploy code. Browser automation, shell execution, file and DB access.' },
  { Icon: Mail,          label: 'Email on autopilot',            desc: 'Reads, drafts, and sends. Inbox zero without touching it.' },
  { Icon: GitBranch,     label: 'Knowledge graph',               desc: 'Long-term memory across sessions. Agents learn from every conversation.' },
  { Icon: MessageSquare, label: 'MCP tool ecosystem',            desc: 'Connect any MCP server — Blender, ComfyUI, Notion, Linear, and beyond.' },
  { Icon: Mic,           label: 'Voice sessions',                desc: 'Talk to any agent in real time with VAD-powered push-to-talk.' },
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

      <p className="text-sm text-secondary-foreground leading-relaxed">
        This wizard configures Qorven on your server. It takes about 3 minutes.
        You'll set up your admin account, connect an AI provider, and optionally wire in channels.
      </p>

      <div>
        <p className="text-xs font-semibold uppercase tracking-widest text-muted-foreground mb-3">
          What you're deploying
        </p>
        <div className="grid grid-cols-2 gap-2">
          {CAPABILITIES.map(({ Icon, label, desc }) => (
            <div key={label} className="flex items-start gap-2.5 rounded-lg px-3 py-2 bg-muted/30 border border-border/50">
              <Icon className="h-3.5 w-3.5 shrink-0 text-primary mt-0.5" />
              <div className="min-w-0">
                <p className="text-xs font-semibold text-foreground leading-tight">{label}</p>
                <p className="text-[11px] text-muted-foreground leading-snug mt-0.5">{desc}</p>
              </div>
            </div>
          ))}
        </div>
      </div>

      <p className="text-xs text-muted-foreground leading-relaxed border-l-2 border-border pl-3">
        All agent actions are governed by per-agent permissions and approval gates.
        Only install Qorven on systems you own or are authorised to manage.
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
