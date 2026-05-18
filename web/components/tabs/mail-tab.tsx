'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { cn } from '@/lib/utils';
import { ComposeSheet } from '@/components/forms/compose-sheet';
import { Inbox, Send, FileEdit, Clock, Settings2, CheckCircle } from 'lucide-react';

interface Props { agentId: string; scope?: 'agent' | 'global' }

const folders = [
  { id: 'inbox', label: 'Inbox', icon: Inbox },
  { id: 'sent', label: 'Sent', icon: Send },
  { id: 'drafts', label: 'Drafts', icon: FileEdit },
  { id: 'scheduled', label: 'Scheduled', icon: Clock },
];

export function MailTab({ agentId, scope = 'agent' }: Props) {
  const [messages, setMessages] = useState<any[]>([]);
  const [activeFolder, setActiveFolder] = useState('inbox');
  const [selected, setSelected] = useState<any>(null);
  const [showCompose, setShowCompose] = useState(false);
  const [identities, setIdentities] = useState<any[]>([]);
  const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

  useEffect(() => {
    fetch(`/api/v1/mail/inbox?agent_id=${agentId}&folder=${activeFolder}`, { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json()).then((d) => setMessages(Array.isArray(d) ? d : [])).catch(() => setMessages([]));
  }, [agentId, activeFolder, getToken()]);

  useEffect(() => {
    fetch(`/api/v1/mail/identities`, { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json()).then((d) => setIdentities(Array.isArray(d) ? d : [])).catch(() => {});
  }, [getToken()]);

  const soulIdentity = identities.find((i: any) => i.agent_id === agentId);

  return (
    <div className="max-w-4xl mx-auto">
      {/* Header */}
      <div className="flex items-center gap-2 mb-4">
        <button onClick={() => setShowCompose(true)} className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90">
          ✉ Compose
        </button>
        <div className="flex-1" />
        {soulIdentity && (
          <span className="flex items-center gap-1.5 text-2xs text-muted-foreground">
            <Settings2 className="h-3 w-3" /> {soulIdentity.address}
          </span>
        )}
      </div>

      <div className="flex gap-4">
        {/* Folder nav */}
        <div className="w-32 shrink-0 space-y-0.5">
          {folders.map((f) => {
            const Icon = f.icon;
            const count = f.id === 'inbox' ? messages.filter((m: any) => !m.is_read).length : 0;
            return (
              <button key={f.id} onClick={() => { setActiveFolder(f.id); setSelected(null); }}
                className={cn('flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs transition-colors',
                  activeFolder === f.id ? 'bg-accent text-foreground font-medium' : 'text-muted-foreground hover:text-foreground')}>
                <Icon className="h-3.5 w-3.5" />
                {f.label}
                {count > 0 && <span className="ml-auto rounded-full bg-primary px-1.5 text-2xs text-primary-foreground">{count}</span>}
              </button>
            );
          })}
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0">
          {selected ? (
            /* Message detail */
            <div className="rounded-xl border border-border p-4">
              <button onClick={() => setSelected(null)} className="text-2xs text-primary hover:underline mb-3">← Back to list</button>
              <div className="flex items-center justify-between mb-2">
                <p className="text-sm font-medium">{selected.subject || '(no subject)'}</p>
                {selected.send_status === 'pending_approval' && (
                  <button className="flex items-center gap-1 rounded-lg bg-emerald-500 px-2.5 py-1 text-xs font-medium text-white hover:bg-emerald-600">
                    <CheckCircle className="h-3 w-3" /> Approve
                  </button>
                )}
              </div>
              <p className="text-2xs text-muted-foreground mb-3">From: {selected.from_address} · {new Date(selected.received_at).toLocaleString()}</p>
              <div className="prose prose-sm prose-invert max-w-none text-sm whitespace-pre-wrap">
                {selected.body_text || selected.body_html || '(empty)'}
              </div>
              <div className="mt-4 flex gap-2">
                <button className="rounded-lg border border-border px-3 py-1.5 text-xs hover:bg-accent">Reply</button>
                <button className="rounded-lg border border-border px-3 py-1.5 text-xs hover:bg-accent">Forward</button>
              </div>
            </div>
          ) : (
            /* Thread list */
            <div className="rounded-xl border border-border divide-y divide-border">
              {messages.length === 0 ? (
                <p className="py-8 text-center text-sm text-muted-foreground">No messages in {activeFolder}</p>
              ) : (
                messages.map((m: any) => (
                  <button key={m.id} onClick={() => setSelected(m)}
                    className={cn('flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-accent/30',
                      !m.is_read && 'bg-accent/10')}>
                    <div className={cn('h-2 w-2 rounded-full shrink-0', m.is_read ? 'bg-transparent' : 'bg-primary')} />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-baseline justify-between gap-2">
                        <p className={cn('text-sm truncate', !m.is_read && 'font-semibold')}>{m.from_name || m.from_address}</p>
                        <span className="text-2xs text-muted-foreground shrink-0">
                          {new Date(m.received_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                        </span>
                      </div>
                      <p className="text-xs text-muted-foreground truncate">{m.subject || '(no subject)'}</p>
                    </div>
                    {m.send_status === 'pending_approval' && (
                      <span className="rounded-full bg-amber-400/10 text-amber-400 px-2 py-0.5 text-2xs font-medium shrink-0">Pending</span>
                    )}
                  </button>
                ))
              )}
            </div>
          )}
        </div>
      </div>
      <ComposeSheet open={showCompose} onClose={() => setShowCompose(false)} agentId={agentId} />
    </div>
  );
}
