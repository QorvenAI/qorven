'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { channels, agents, pairing } from '@/lib/api';
import { ErrorBoundary } from '@/components/error-boundary';
import { ChannelConnectForm } from '@/components/channels/channel-connect-form';
import { channelFormSchemas, CHANNEL_TYPES } from '@/components/channels/channel-schemas';
import { AlertCircle, Plus, Power, Pencil, Trash2, Loader2, Zap, ChevronRight, Search, Info } from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { cn } from '@/lib/utils';
import type { Channel, ChannelType, Soul } from '@/types';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { BrandIcon } from '@/components/brand-icon';
import { toast } from 'sonner';

interface PairedDevice {
  id: string;
  channel_type: string;
  sender_id: string;
  chat_id: string;
  sender_name: string;
  paired_at: string;
}

type DrawerState =
  | { mode: 'add'; agentId: string; type: ChannelType }
  | { mode: 'edit'; channel: Channel }
  | null;

export default function ChannelsPage() {
  const [list, setList] = useState<Channel[]>([]);
  const [agentList, setAgentList] = useState<Soul[]>([]);
  const [devices, setDevices] = useState<PairedDevice[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [toggling, setToggling] = useState<string | null>(null);
  const [drawer, setDrawer] = useState<DrawerState>(null);
  const [showAddPicker, setShowAddPicker] = useState(false);
  const [pickerSearch, setPickerSearch] = useState('');
  const [activeAgentId, setActiveAgentId] = useState<string>('');
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  const load = () => {
    setLoading(true);
    setError(null);
    Promise.all([channels.list(), agents.list(), pairing.devices().catch(() => [])])
      .then(([chs, ags, devs]) => { setList(chs); setAgentList(ags); setDevices(devs as PairedDevice[]); setLoading(false); })
      .catch((e) => { setError(e.message); setLoading(false); });
  };

  useEffect(load, []);

  const toggle = async (ch: Channel) => {
    setToggling(ch.id);
    try {
      if (ch.status === 'running') {
        await channels.stop(ch.id);
        toast.success(`${ch.name || ch.channel_type} stopped`);
      } else {
        await channels.start(ch.id);
        toast.success(`${ch.name || ch.channel_type} started`);
      }
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to toggle channel');
    } finally {
      setToggling(null);
    }
  };

  const handleDelete = async (ch: Channel) => {
    if (!confirm(`Delete channel "${ch.name || ch.channel_type}"?`)) return;
    try {
      await channels.delete(ch.id);
      toast.success(`${ch.name || ch.channel_type} deleted`);
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to delete channel');
    }
  };

  const agentName = (id: string) => {
    const a = agentList.find((a) => a.id === id);
    return a?.display_name ?? id.slice(0, 8);
  };

  const scopedList = activeAgentId === ''
    ? list
    : list.filter((ch) => ch.agent_id === activeAgentId);

  const countByAgent = (agentId: string) =>
    list.filter((ch) => ch.agent_id === agentId).length;

  const groupedByAgent = agentList
    .filter((a) => list.some((ch) => ch.agent_id === a.id))
    .map((a) => ({ agent: a, channels: list.filter((ch) => ch.agent_id === a.id) }));

  const toggleCollapse = (agentId: string) =>
    setCollapsed((prev) => ({ ...prev, [agentId]: !prev[agentId] }));

  const filteredPickerTypes = CHANNEL_TYPES.filter((t) =>
    pickerSearch === '' || channelFormSchemas[t].label.toLowerCase().includes(pickerSearch.toLowerCase())
  );

  return (
    <ErrorBoundary fallbackTitle="Failed to load channels">
      <div className="space-y-4">
        <CanvasHeader
          title="Channels"
          description={loading ? 'Loading…' : `${list.length} channel${list.length !== 1 ? 's' : ''} configured`}
          actions={
            <button
              onClick={() => { setShowAddPicker(true); setPickerSearch(''); }}
              className="flex items-center gap-2 rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              <Plus className="h-4 w-4" />
              Add Channel
            </button>
          }
        />

        {/* Agent tab strip */}
        {!loading && agentList.length > 0 && (
          <div className="flex gap-2 overflow-x-auto pb-1 [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden">
            <button
              onClick={() => setActiveAgentId('')}
              className={cn(
                'shrink-0 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
                activeAgentId === ''
                  ? 'bg-primary text-primary-foreground'
                  : 'border border-border text-muted-foreground hover:bg-accent'
              )}
            >
              All
              <span className="ml-1.5 text-xs opacity-70">{list.length}</span>
            </button>
            {agentList.map((a) => (
              <button
                key={a.id}
                onClick={() => setActiveAgentId(a.id)}
                className={cn(
                  'shrink-0 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors whitespace-nowrap',
                  activeAgentId === a.id
                    ? 'bg-primary text-primary-foreground'
                    : 'border border-border text-muted-foreground hover:bg-accent'
                )}
              >
                {a.display_name}
                {countByAgent(a.id) > 0 && (
                  <span className="ml-1.5 text-xs opacity-70">{countByAgent(a.id)}</span>
                )}
              </button>
            ))}
          </div>
        )}

        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="rounded-xl border border-border bg-card px-4 py-3 flex items-center gap-3">
                <div className="h-8 w-8 animate-pulse rounded-lg bg-muted shrink-0" />
                <div className="flex-1 space-y-1.5">
                  <div className="h-4 w-32 animate-pulse rounded bg-muted" />
                  <div className="h-3 w-24 animate-pulse rounded bg-muted" />
                </div>
                <div className="h-5 w-16 animate-pulse rounded-full bg-muted" />
              </div>
            ))}
          </div>
        ) : error ? (
          <div className="flex flex-col items-center py-16 text-center">
            <AlertCircle className="h-8 w-8 text-destructive" />
            <p className="mt-2 text-sm text-destructive">{error}</p>
            <button onClick={load} className="mt-3 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">
              Retry
            </button>
          </div>
        ) : list.length === 0 ? (
          <EmptyState
            {...emptyStates.channels}
            description="No channels configured yet. Add a channel like Telegram, Discord, or WhatsApp to connect your agents."
          />
        ) : activeAgentId === '' ? (
          /* "All" tab — grouped by agent (A3) */
          <div className="space-y-4">
            {groupedByAgent.map(({ agent, channels: agentChannels }) => (
              <div key={agent.id}>
                <button
                  onClick={() => toggleCollapse(agent.id)}
                  className="flex items-center gap-2 w-full py-1.5 text-sm font-medium text-muted-foreground hover:text-foreground transition-colors"
                >
                  <ChevronRight className={cn('h-3.5 w-3.5 transition-transform', !collapsed[agent.id] && 'rotate-90')} />
                  {agent.display_name}
                  <span className="text-xs opacity-60 ml-1">{agentChannels.length} channel{agentChannels.length !== 1 ? 's' : ''}</span>
                </button>
                {!collapsed[agent.id] && (
                  <div className="space-y-1.5 mt-1">
                    {agentChannels.map((ch) => (
                      <ChannelRow
                        key={ch.id}
                        ch={ch}
                        agentName={agentName(ch.agent_id)}
                        isToggling={toggling === ch.id}
                        devices={devices.filter((d) => d.channel_type === ch.channel_type)}
                        onEdit={() => setDrawer({ mode: 'edit', channel: ch })}
                        onToggle={() => toggle(ch)}
                        onDelete={() => handleDelete(ch)}
                      />
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        ) : (
          /* Scoped tab — flat list */
          <div className="space-y-1.5">
            {scopedList.length === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">No channels for this agent yet.</p>
            ) : (
              scopedList.map((ch) => (
                <ChannelRow
                  key={ch.id}
                  ch={ch}
                  agentName={agentName(ch.agent_id)}
                  isToggling={toggling === ch.id}
                  devices={devices.filter((d) => d.channel_type === ch.channel_type)}
                  onEdit={() => setDrawer({ mode: 'edit', channel: ch })}
                  onToggle={() => toggle(ch)}
                  onDelete={() => handleDelete(ch)}
                />
              ))
            )}
          </div>
        )}

        {/* Channel type picker modal */}
        {showAddPicker && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
            onClick={(e) => { if (e.target === e.currentTarget) setShowAddPicker(false); }}
            onKeyDown={(e) => { if (e.key === 'Escape') setShowAddPicker(false); }}
          >
            <div className="w-full max-w-lg rounded-2xl border border-border bg-background p-6 max-h-[80vh] flex flex-col">
              <h2 className="mb-3 text-base font-semibold">Choose Channel Type</h2>
              <div className="relative mb-3">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
                <input
                  type="text"
                  value={pickerSearch}
                  onChange={(e) => setPickerSearch(e.target.value)}
                  placeholder="Search channel types…"
                  className="qr-input pl-8 text-sm"
                  autoFocus
                />
              </div>
              <div className="overflow-y-auto">
                <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
                  {filteredPickerTypes.map((type) => {
                    const schema = channelFormSchemas[type];
                    return (
                      <button
                        key={type}
                        onClick={() => {
                          setShowAddPicker(false);
                          if (!activeAgentId) {
                            toast.error('Select an agent tab before adding a channel');
                            return;
                          }
                          setDrawer({ mode: 'add', agentId: activeAgentId, type });
                        }}
                        className="flex flex-col items-center gap-1.5 rounded-xl border border-border p-3 text-center hover:border-primary/40 hover:bg-accent transition-colors"
                      >
                        <span className={cn('flex h-9 w-9 items-center justify-center rounded-lg', schema.color)}>
                          <BrandIcon name={type} size={20} useBrandColor={false} />
                        </span>
                        <p className="text-xs font-medium leading-tight">{schema.label}</p>
                      </button>
                    );
                  })}
                </div>
                {filteredPickerTypes.length === 0 && (
                  <p className="py-6 text-center text-sm text-muted-foreground">No channel types match &quot;{pickerSearch}&quot;</p>
                )}
              </div>
              <button
                onClick={() => setShowAddPicker(false)}
                className="mt-4 w-full rounded-lg border border-border py-2 text-sm text-muted-foreground hover:bg-accent"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {/* Connect / Edit drawer */}
        {drawer?.mode === 'add' && (
          <ChannelConnectForm
            agentId={drawer.agentId}
            channelType={drawer.type}
            onClose={() => setDrawer(null)}
            onConnected={() => { setDrawer(null); load(); toast.success('Channel connected'); }}
          />
        )}
        {drawer?.mode === 'edit' && (
          <ChannelConnectForm
            agentId={drawer.channel.agent_id}
            channelType={drawer.channel.channel_type}
            existing={drawer.channel}
            onClose={() => setDrawer(null)}
            onConnected={() => { setDrawer(null); load(); toast.success('Channel updated'); }}
          />
        )}
      </div>
    </ErrorBoundary>
  );
}

interface ChannelRowProps {
  ch: Channel;
  agentName: string;
  isToggling: boolean;
  devices: PairedDevice[];
  onEdit: () => void;
  onToggle: () => void;
  onDelete: () => void;
}

function ChannelRow({ ch, agentName, isToggling, devices, onEdit, onToggle, onDelete }: ChannelRowProps) {
  const schema = channelFormSchemas[ch.channel_type];
  const [showDevices, setShowDevices] = useState(false);
  const hasDevices = ch.status === 'running' && devices.length > 0;

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="flex items-center gap-3 px-4 py-3">
        <span className={cn('flex h-8 w-8 shrink-0 items-center justify-center rounded-lg', schema?.color)}>
          <BrandIcon name={ch.channel_type} size={18} useBrandColor={false} />
        </span>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium truncate">{ch.name || schema?.label || ch.channel_type}</p>
          <p className="text-xs text-muted-foreground">{schema?.label ?? ch.channel_type} · {agentName}</p>
        </div>
        <span className={cn(
          'flex items-center gap-1 text-xs',
          ch.status === 'running' ? 'text-emerald-500' : 'text-muted-foreground'
        )}>
          <Zap className="h-3 w-3" />
          {ch.status === 'running' ? 'Running' : ch.status || 'Stopped'}
        </span>
        <div className="flex items-center gap-1 shrink-0">
          {hasDevices && (
            <button
              onClick={() => setShowDevices((v) => !v)}
              aria-label="Show connected users"
              title={`${devices.length} connected user${devices.length !== 1 ? 's' : ''}`}
              className={cn(
                'flex h-7 w-7 items-center justify-center rounded-md transition-colors',
                showDevices ? 'text-primary bg-primary/10' : 'text-muted-foreground hover:bg-accent'
              )}
            >
              <Info className="h-3.5 w-3.5" />
            </button>
          )}
          <button onClick={onEdit} aria-label="Edit channel" className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:bg-accent" title="Edit">
            <Pencil className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={onToggle}
            disabled={isToggling}
            className={cn('flex h-7 w-7 items-center justify-center rounded-md',
              ch.status === 'running' ? 'text-emerald-500 hover:bg-accent' : 'text-muted-foreground hover:bg-accent')}
            aria-label={ch.status === 'running' ? 'Stop channel' : 'Start channel'}
            title={ch.status === 'running' ? 'Stop' : 'Start'}
          >
            {isToggling ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Power className="h-3.5 w-3.5" />}
          </button>
          <button onClick={onDelete} aria-label="Delete channel" className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-destructive hover:bg-destructive/10" title="Delete">
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
      {showDevices && (
        <div className="border-t border-border bg-muted/30 px-4 py-2 space-y-1.5">
          <p className="text-2xs text-muted-foreground font-medium uppercase tracking-wide mb-1">Connected Users</p>
          {devices.map((d) => {
            const relativeTime = (() => {
              const diff = Date.now() - new Date(d.paired_at).getTime();
              const mins = Math.floor(diff / 60000);
              if (mins < 60) return `${mins}m ago`;
              const hrs = Math.floor(mins / 60);
              if (hrs < 24) return `${hrs}h ago`;
              return new Date(d.paired_at).toLocaleDateString();
            })();
            return (
              <div key={d.id} className="flex items-center gap-2 py-1">
                <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded bg-background border border-border">
                  <BrandIcon name={d.channel_type} size={12} />
                </span>
                <span className="text-xs font-medium truncate">{d.sender_name || 'Unknown'}</span>
                <span className="text-xs text-muted-foreground truncate">{formatSenderId(d.channel_type, d.sender_id)}</span>
                <span className="ml-auto text-xs text-muted-foreground shrink-0">{relativeTime}</span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function formatSenderId(channelType: string, senderId: string): string {
  if (channelType === 'telegram') return `ID: ${senderId}`;
  if (channelType === 'whatsapp') {
    // WhatsApp sender IDs are often phone@c.us or phone@s.whatsapp.net
    const phone = senderId.replace(/@.*/, '');
    return phone.startsWith('+') ? phone : `+${phone}`;
  }
  if (channelType === 'discord') return `Discord: ${senderId}`;
  if (channelType === 'slack') return `Slack: ${senderId}`;
  return senderId;
}

