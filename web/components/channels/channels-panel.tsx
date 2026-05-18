'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { channels as channelsApi } from '@/lib/api';
import { ChannelBadge } from '@/components/channel-badge';
import { ChannelConnectForm } from '@/components/channels/channel-connect-form';
import { channelFormSchemas } from '@/components/channels/channel-schemas';
import { cn } from '@/lib/utils';
import { Plus, Power, Pencil, Trash2, Loader2 } from 'lucide-react';
import { BrandIcon } from '@/components/brand-icon';
import type { Channel, ChannelType } from '@/types';

interface ChannelsPanelProps {
  agentId: string;
  channels: Channel[];
  onRefresh: () => void;
}

type DrawerState =
  | { mode: 'add'; type: ChannelType }
  | { mode: 'edit'; channel: Channel }
  | null;

export function ChannelsPanel({ agentId, channels, onRefresh }: ChannelsPanelProps) {
  const [drawer, setDrawer] = useState<DrawerState>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  const handleToggle = async (ch: Channel) => {
    setActionLoading(ch.id);
    try {
      if (ch.status === 'running') {
        await channelsApi.stop(ch.id);
      } else {
        await channelsApi.start(ch.id);
      }
      onRefresh();
    } catch { /* toast handled upstream */ }
    setActionLoading(null);
  };

  const handleDelete = async (ch: Channel) => {
    if (!confirm(`Disconnect "${ch.name}"?`)) return;
    setActionLoading(ch.id);
    try {
      await channelsApi.delete(ch.id);
      onRefresh();
    } catch { /* toast handled upstream */ }
    setActionLoading(null);
  };

  const allTypes = Object.keys(channelFormSchemas) as ChannelType[];
  const connectedTypes = new Set(channels.map((c) => c.channel_type));

  return (
    <div className="space-y-6">
      {/* Connected channels */}
      {channels.length > 0 && (
        <div className="space-y-2">
          {channels.map((ch) => {
            const schema = channelFormSchemas[ch.channel_type];
            const isLoading = actionLoading === ch.id;
            return (
              <div key={ch.id} className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3">
                <ChannelBadge type={ch.channel_type} status={ch.status === 'running' ? 'live' : 'offline'} />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium">{ch.name}</p>
                  <p className="text-2xs text-muted-foreground">{schema?.description ?? ch.channel_type} · {ch.status}</p>
                </div>
                <button
                  onClick={() => setDrawer({ mode: 'edit', channel: ch })}
                  className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent"
                  title="Edit configuration"
                >
                  <Pencil className="h-3.5 w-3.5" />
                </button>
                <button
                  onClick={() => handleToggle(ch)}
                  disabled={isLoading}
                  className={cn('flex h-8 w-8 items-center justify-center rounded-lg',
                    ch.status === 'running' ? 'text-emerald-500 hover:bg-accent' : 'text-muted-foreground hover:bg-accent')}
                  title={ch.status === 'running' ? 'Stop' : 'Start'}
                >
                  {isLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Power className="h-4 w-4" />}
                </button>
                <button
                  onClick={() => handleDelete(ch)}
                  disabled={isLoading}
                  className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent hover:text-destructive"
                  title="Delete"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            );
          })}
        </div>
      )}

      {/* Add channel grid */}
      <div>
        <h3 className="mb-3 text-sm font-medium text-muted-foreground uppercase tracking-wide text-2xs">Add Channel</h3>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
          {allTypes.map((type) => {
            const schema = channelFormSchemas[type];
            const connected = connectedTypes.has(type);
            return (
              <button
                key={type}
                onClick={() => !connected && setDrawer({ mode: 'add', type })}
                disabled={connected}
                className={cn(
                  'flex flex-col items-center gap-2 rounded-xl border border-border p-3 text-center transition-colors',
                  connected ? 'opacity-40 cursor-default' : 'hover:border-primary/30 hover:bg-accent cursor-pointer',
                )}
              >
                <span className={cn('flex h-8 w-8 items-center justify-center rounded-lg', schema.color)}>
                  <BrandIcon name={type} size={18} useBrandColor={false} />
                </span>
                <span className="text-2xs font-medium leading-tight">{schema.label}</span>
                {connected && <span className="text-2xs text-emerald-500">Connected</span>}
              </button>
            );
          })}
        </div>
      </div>

      {/* Drawer */}
      {drawer?.mode === 'add' && (
        <ChannelConnectForm
          agentId={agentId}
          channelType={drawer.type}
          onClose={() => setDrawer(null)}
          onConnected={() => { setDrawer(null); onRefresh(); }}
        />
      )}
      {drawer?.mode === 'edit' && (
        <ChannelConnectForm
          agentId={agentId}
          channelType={drawer.channel.channel_type}
          existing={drawer.channel}
          onClose={() => setDrawer(null)}
          onConnected={() => { setDrawer(null); onRefresh(); }}
        />
      )}
    </div>
  );
}
