'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import { agents as agentsApi, sessions as sessionsApi } from '@/lib/api';
import { ChatPlayground } from '@/components/chat-v2/chat-playground';
import { Loader2, AlertCircle, Bot, ChevronDown } from 'lucide-react';
import type { Soul, Session } from '@/types';
import { cn } from '@/lib/utils';

export function ChatClient() {
  const [agentList, setAgentList] = useState<Soul[]>([]);
  const [selectedAgent, setSelectedAgent] = useState<Soul | null>(null);
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [agentOpen, setAgentOpen] = useState(false);

  // Load agents on mount
  useEffect(() => {
    let active = true;
    agentsApi.list()
      .then((list) => {
        if (!active) return;
        setAgentList(list);
        if (list.length > 0) setSelectedAgent(list[0] ?? null);
      })
      .catch((e) => { if (active) setError(e instanceof Error ? e.message : 'Failed to load agents'); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, []);

  // Create/get session when agent changes
  useEffect(() => {
    if (!selectedAgent) return;
    let active = true;
    setSession(null);
    setError(null);
    sessionsApi.create({ agent_id: selectedAgent.id, channel: 'web' })
      .then((s) => { if (active) setSession(s); })
      .catch((e) => { if (active) setError(e instanceof Error ? e.message : 'Failed to start session'); });
    return () => { active = false; };
  }, [selectedAgent]);

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
        <AlertCircle className="h-8 w-8 text-destructive" />
        <p className="text-sm text-muted-foreground">{error}</p>
        <button onClick={() => window.location.reload()} className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">
          Retry
        </button>
      </div>
    );
  }

  if (!selectedAgent || !session) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col relative">
      {/* Agent selector bar */}
      <div className="flex items-center gap-2 border-b border-border px-4 py-2 bg-background/80 backdrop-blur-sm">
        <Bot className="h-4 w-4 text-muted-foreground shrink-0" />
        <div className="relative">
          <button
            onClick={() => setAgentOpen(!agentOpen)}
            className="flex items-center gap-1.5 rounded-lg px-2 py-1 text-sm font-medium hover:bg-accent transition-colors"
          >
            {selectedAgent.display_name}
            <ChevronDown className={cn('h-3.5 w-3.5 text-muted-foreground transition-transform', agentOpen && 'rotate-180')} />
          </button>
          {agentOpen && (
            <div className="absolute left-0 top-full mt-1 z-50 min-w-[200px] rounded-xl border border-border bg-popover shadow-lg overflow-hidden">
              {agentList.map((ag) => (
                <button
                  key={ag.id}
                  onClick={() => { setSelectedAgent(ag); setAgentOpen(false); }}
                  className={cn(
                    'flex w-full items-center gap-2 px-3 py-2 text-sm hover:bg-accent transition-colors text-left',
                    ag.id === selectedAgent.id && 'text-primary font-medium',
                  )}
                >
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-primary/10 text-primary text-2xs font-bold shrink-0">
                    {(ag.display_name || 'A')[0]?.toUpperCase()}
                  </span>
                  {ag.display_name}
                </button>
              ))}
            </div>
          )}
        </div>
        <div className="flex-1" />
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 inline-block" />
          <span>Live</span>
        </div>
      </div>

      {/* Chat playground */}
      <ChatPlayground
        agentId={selectedAgent.id}
        sessionId={session.id}
        agentName={selectedAgent.display_name}
        initialThinkingLevel={(selectedAgent.thinking_level as 'off' | 'medium' | 'high') || 'off'}
        className="flex-1 min-h-0 relative"
      />

      {/* Click outside to close agent picker */}
      {agentOpen && (
        <div className="fixed inset-0 z-40" onClick={() => setAgentOpen(false)} />
      )}
    </div>
  );
}
