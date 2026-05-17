'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect } from 'react';
import { cn } from '@/lib/utils';
import { ChannelsPanel } from '@/components/channels/channels-panel';
import { ConnectorsPanel } from '@/components/connectors/connectors-panel';
import { agents, channels as channelsApi } from '@/lib/api';
import { Link2, Plug, Wrench, Sparkles } from 'lucide-react';
import type { Channel, Soul } from '@/types';

interface Props { agentId: string; soul: Soul }

const tabs = [
  { id: 'channels', label: 'Channels', icon: Link2 },
  { id: 'connectors', label: 'Connectors', icon: Plug },
  { id: 'tools', label: 'Tools', icon: Wrench },
  { id: 'skills', label: 'Skills', icon: Sparkles },
] as const;

type TabId = (typeof tabs)[number]['id'];

export function IntegrationsTab({ agentId, soul }: Props) {
  const [active, setActive] = useState<TabId>('channels');
  const [soulChannels, setSoulChannels] = useState<Channel[]>([]);
  const [skills, setSkills] = useState<any[]>([]);

  useEffect(() => {
    channelsApi.list().then((c) => setSoulChannels(c.filter((x: any) => x.agent_id === agentId))).catch(() => {});
    agents.skills(agentId).then(setSkills).catch(() => {});
  }, [agentId]);

  const allowed = soul.tools_allowed ?? [];
  const denied = soul.tools_denied ?? [];

  return (
    <div className="space-y-6 max-w-3xl mx-auto">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Integrations</h1>
        <p className="text-sm text-muted-foreground">Channels, connectors, tools and skills for {soul.display_name}</p>
      </div>

      <div className="flex gap-6">
        {/* Sidebar nav — same as Settings */}
        <nav className="hidden w-48 shrink-0 space-y-1 lg:block">
          {tabs.map(({ id, label, icon: Icon }) => (
            <button key={id} onClick={() => setActive(id)}
              className={cn('flex w-full items-center gap-2 rounded-lg px-3 py-2 text-2sm font-medium transition-colors',
                active === id ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground')}>
              <Icon className="h-4 w-4" /> {label}
            </button>
          ))}
        </nav>

        {/* Content */}
        <div className="flex-1 min-w-0">
          {active === 'channels' && (
            <ChannelsPanel agentId={agentId} channels={soulChannels}
              onRefresh={() => channelsApi.list().then((c) => setSoulChannels(c.filter((x: any) => x.agent_id === agentId)))} />
          )}
          {active === 'connectors' && <ConnectorsPanel agentId={agentId} />}
          {active === 'tools' && (
            <div className="space-y-4">
              <Section title="Tool Profile" description={`Profile: ${soul.tool_profile || 'full'}`}>
                {allowed.length > 0 && (
                  <div>
                    <p className="text-2xs text-muted-foreground mb-1">Allowed</p>
                    <div className="flex flex-wrap gap-1">{allowed.map((t: string) => <span key={t} className="rounded-md bg-emerald-400/10 text-emerald-400 px-2 py-0.5 text-2xs">{t}</span>)}</div>
                  </div>
                )}
                {denied.length > 0 && (
                  <div>
                    <p className="text-2xs text-muted-foreground mb-1">Denied</p>
                    <div className="flex flex-wrap gap-1">{denied.map((t: string) => <span key={t} className="rounded-md bg-destructive/10 text-destructive px-2 py-0.5 text-2xs">{t}</span>)}</div>
                  </div>
                )}
                {allowed.length === 0 && denied.length === 0 && <p className="text-xs text-muted-foreground">All tools available</p>}
              </Section>
            </div>
          )}
          {active === 'skills' && (
            <div className="space-y-4">
              <Section title="Installed Skills" description={`${skills.length} skill${skills.length !== 1 ? 's' : ''} installed`}>
                {skills.length === 0 ? <p className="text-xs text-muted-foreground">No skills installed</p> :
                  <div className="grid gap-2 sm:grid-cols-2">{skills.map((s: any) => (
                    <div key={s.slug || s.id} className="rounded-lg border border-border p-3">
                      <p className="text-sm font-medium">{s.name}</p>
                      <p className="text-2xs text-muted-foreground mt-1">{s.description?.slice(0, 80)}</p>
                    </div>
                  ))}</div>}
              </Section>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function Section({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
  return (
    <div>
      <h3 className="text-lg font-medium">{title}</h3>
      {description && <p className="text-sm text-muted-foreground mb-4">{description}</p>}
      <div className="space-y-4">{children}</div>
    </div>
  );
}
