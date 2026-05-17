'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// A2A is the agent-to-agent protocol: peer Qorven instances can discover
// this platform's agents via the well-known URLs and call them as remote skills.
// Informational only — inbound tokens are ordinary user API keys managed elsewhere.

import { useEffect, useState } from 'react';
import { Loader2, Network, AlertTriangle, Copy, Check, ExternalLink } from 'lucide-react';
import { a2a, type A2AAgentCard } from '@/lib/api';
import { Card, Row } from './primitives';

export function A2AFederationCard() {
  const [card, setCard] = useState<A2AAgentCard | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [copiedUrl, setCopiedUrl] = useState(false);

  useEffect(() => {
    a2a.platformCard()
      .then(setCard)
      .catch((e) => setErr(e instanceof Error ? e.message : 'Not reachable'))
      .finally(() => setLoading(false));
  }, []);

  const platformUrl = typeof window !== 'undefined'
    ? `${window.location.origin}${a2a.platformCardUrl()}`
    : a2a.platformCardUrl();

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(platformUrl);
      setCopiedUrl(true);
      setTimeout(() => setCopiedUrl(false), 1500);
    } catch { /* clipboard blocked */ }
  };

  return (
    <Card
      id="a2a_federation"
      title="A2A Federation"
      description="Peer Qorven instances (and other A2A-compatible platforms) can discover and call this workspace's agents via these public endpoints."
    >
      <div className="space-y-3">
        <Row label="Platform card URL" hint="Publish this to let others discover your roster">
          <div className="flex items-center gap-2">
            <code className="flex-1 rounded-md bg-muted px-2 py-1 font-mono text-xs">{platformUrl}</code>
            <button onClick={copy}
              className="flex h-7 w-7 items-center justify-center rounded-md border border-border text-muted-foreground hover:bg-accent"
              title="Copy">
              {copiedUrl ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
            </button>
            <a href={platformUrl} target="_blank" rel="noreferrer noopener"
              className="flex h-7 w-7 items-center justify-center rounded-md border border-border text-muted-foreground hover:bg-accent"
              title="Open">
              <ExternalLink className="h-3.5 w-3.5" />
            </a>
          </div>
        </Row>

        {loading && (
          <p className="flex items-center gap-2 text-xs text-muted-foreground">
            <Loader2 className="h-3.5 w-3.5 animate-spin" /> Probing endpoint…
          </p>
        )}

        {err && (
          <p className="text-xs text-destructive">
            <AlertTriangle className="mr-1 inline h-3.5 w-3.5" />
            {err} — the A2A router may not be enabled. Check your gateway config.
          </p>
        )}

        {card && (
          <div className="rounded-lg border border-border bg-card p-3 text-xs space-y-2">
            <div className="flex items-center gap-2">
              <Network className="h-3.5 w-3.5 text-primary" />
              <span className="font-semibold">{card.name}</span>
              {card.version && (
                <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground">
                  v{card.version}
                </span>
              )}
            </div>
            {card.description && <p className="text-muted-foreground">{card.description}</p>}
            {card.skills && card.skills.length > 0 && (
              <div>
                <p className="text-2xs font-medium uppercase tracking-wider text-muted-foreground">
                  Exposed skills ({card.skills.length})
                </p>
                <div className="mt-1 flex flex-wrap gap-1">
                  {card.skills.slice(0, 20).map((s) => (
                    <span key={s.id} className="rounded-sm bg-primary/10 px-1.5 py-0.5 font-mono text-xs text-primary">
                      {s.name}
                    </span>
                  ))}
                  {card.skills.length > 20 && (
                    <span className="text-2xs text-muted-foreground">+{card.skills.length - 20} more</span>
                  )}
                </div>
              </div>
            )}
            {card.authentication?.schemes && card.authentication.schemes.length > 0 && (
              <p className="text-2xs text-muted-foreground">
                Inbound auth: {card.authentication.schemes.map((s) => s.scheme).join(', ')}
                {' '}— use a regular user API key as the bearer token.
              </p>
            )}
          </div>
        )}
      </div>
    </Card>
  );
}
