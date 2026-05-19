'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import {
  Bot, Tag, Globe, ExternalLink, Loader2, RefreshCw, Copy, Check,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { cn } from '@/lib/utils';
import { BASE } from '@/lib/api-core';

interface A2ASkill {
  id: string;
  name: string;
  description?: string;
  tags?: string[];
}

interface A2ACard {
  name: string;
  description?: string;
  url?: string;
  version?: string;
  documentation_url?: string;
  skills?: A2ASkill[];
  capabilities?: Record<string, boolean>;
  default_input_modes?: string[];
  default_output_modes?: string[];
}

export default function A2APage() {
  const [card, setCard] = useState<A2ACard | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const refresh = async () => {
    setLoading(true);
    setErr(null);
    try {
      // The A2A platform card is at /a2a/.well-known/agent.json (public, no auth)
      const res = await fetch('/api/a2a/.well-known/agent.json');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setCard(await res.json());
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to load A2A card');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { refresh(); }, []);

  const cardUrl = typeof window !== 'undefined'
    ? `${window.location.origin}/api/a2a/.well-known/agent.json`
    : '/api/a2a/.well-known/agent.json';

  const copyUrl = async () => {
    await navigator.clipboard.writeText(cardUrl);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="mx-auto max-w-4xl space-y-5 p-4 lg:p-6">
      <CanvasHeader
        title="Agent-to-Agent (A2A)"
        description="Qorven exposes an A2A endpoint for external agents to discover and delegate tasks."
        actions={
          <button
            onClick={refresh}
            disabled={loading}
            className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-accent disabled:opacity-60"
          >
            {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
            Refresh
          </button>
        }
      />

      {/* Discovery URL */}
      <div className="rounded-xl border border-border bg-card p-4">
        <h2 className="text-sm font-semibold mb-2 flex items-center gap-2">
          <Globe className="h-4 w-4 text-muted-foreground" />
          Discovery URL
        </h2>
        <div className="flex items-center gap-2">
          <code className="flex-1 rounded-md bg-muted px-3 py-2 text-xs font-mono truncate">
            {cardUrl}
          </code>
          <button
            onClick={copyUrl}
            className="flex h-8 w-8 items-center justify-center rounded-md border border-border hover:bg-accent text-muted-foreground hover:text-foreground"
          >
            {copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
          </button>
          <a
            href={cardUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="flex h-8 w-8 items-center justify-center rounded-md border border-border hover:bg-accent text-muted-foreground hover:text-foreground"
          >
            <ExternalLink className="h-3.5 w-3.5" />
          </a>
        </div>
        <p className="mt-2 text-2xs text-muted-foreground">
          Share this URL with any A2A-compatible agent to connect it to your Qorven team.
        </p>
      </div>

      {err && (
        <div className="rounded-xl border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          {err} — the A2A server may not be initialized on this instance.
        </div>
      )}

      {loading && (
        <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading agent card…
        </div>
      )}

      {card && (
        <>
          {/* Platform card */}
          <div className="rounded-xl border border-border bg-card p-4 space-y-3">
            <div className="flex items-start gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">
                <Bot className="h-5 w-5" />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <h3 className="text-sm font-semibold">{card.name}</h3>
                  {card.version && (
                    <span className="font-mono text-2xs text-muted-foreground">v{card.version}</span>
                  )}
                </div>
                {card.description && (
                  <p className="mt-1 text-xs text-muted-foreground">{card.description}</p>
                )}
              </div>
              {card.documentation_url && (
                <a
                  href={card.documentation_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-2xs text-primary hover:underline flex items-center gap-1"
                >
                  Docs <ExternalLink className="h-2.5 w-2.5" />
                </a>
              )}
            </div>

            {card.capabilities && Object.keys(card.capabilities).length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(card.capabilities).map(([k, v]) => (
                  <span
                    key={k}
                    className={cn(
                      'rounded-full px-2 py-0.5 text-2xs font-mono',
                      v ? 'bg-emerald-500/10 text-emerald-500' : 'bg-muted text-muted-foreground line-through',
                    )}
                  >
                    {k}
                  </span>
                ))}
              </div>
            )}

            {(card.default_input_modes || card.default_output_modes) && (
              <div className="flex items-center gap-3 text-2xs text-muted-foreground">
                {card.default_input_modes && (
                  <span>in: {card.default_input_modes.join(', ')}</span>
                )}
                {card.default_output_modes && (
                  <span>out: {card.default_output_modes.join(', ')}</span>
                )}
              </div>
            )}
          </div>

          {/* Skills */}
          {card.skills && card.skills.length > 0 && (
            <section className="rounded-xl border border-border bg-card/40">
              <header className="flex items-center gap-2 border-b border-border/60 px-4 py-2.5">
                <Tag className="h-4 w-4 text-muted-foreground" />
                <h2 className="text-sm font-semibold">Skills</h2>
                <span className="ml-auto font-mono text-2xs text-muted-foreground">{card.skills.length}</span>
              </header>
              <div className="divide-y divide-border/40">
                {card.skills.map((skill) => (
                  <div key={skill.id} className="flex items-start gap-3 px-4 py-3">
                    <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary/10 text-primary shrink-0">
                      <Bot className="h-3.5 w-3.5" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium">{skill.name}</span>
                        <span className="font-mono text-2xs text-muted-foreground">{skill.id}</span>
                      </div>
                      {skill.description && (
                        <p className="mt-0.5 text-xs text-muted-foreground">{skill.description}</p>
                      )}
                      {skill.tags && skill.tags.length > 0 && (
                        <div className="mt-1.5 flex flex-wrap gap-1">
                          {skill.tags.map((t) => (
                            <span key={t} className="rounded-full bg-muted px-1.5 py-0.5 text-2xs text-muted-foreground">
                              {t}
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </section>
          )}
        </>
      )}

      {/* How to use */}
      <section className="rounded-xl border border-border bg-card/40 p-4 space-y-3">
        <h2 className="text-sm font-semibold">How to delegate tasks via A2A</h2>
        <div className="space-y-2 text-xs text-muted-foreground">
          <p>1. Share the discovery URL above with an external agent or framework.</p>
          <p>2. The external agent reads the agent card to discover available skills and endpoints.</p>
          <p>3. It creates a task by POSTing to <code className="font-mono bg-muted px-1 rounded">/api/a2a/agents/&#123;key&#125;/tasks</code></p>
          <p>4. Results are polled via <code className="font-mono bg-muted px-1 rounded">GET /api/a2a/tasks/&#123;id&#125;</code></p>
        </div>
      </section>
    </div>
  );
}
