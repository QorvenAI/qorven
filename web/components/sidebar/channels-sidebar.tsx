'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { ShieldCheck } from 'lucide-react';
import { channels as channelsApi } from '@/lib/api';
import { BrandIcon } from '@/components/brand-icon';
import { SidebarGroupTitle, SidebarMenuItem, SidebarDivider } from './sidebar-primitives';

export function ChannelsSidebar() {
  const [channels, setChannels] = useState<any[]>([]);
  const router = useRouter();
  useEffect(() => { channelsApi.list().then(setChannels).catch(() => {}); }, []);
  const grouped = channels.reduce((acc: Record<string, any[]>, ch) => {
    acc[ch.channel_type] = acc[ch.channel_type] || [];
    acc[ch.channel_type]!.push(ch);
    return acc;
  }, {});
  const pathname = typeof window !== 'undefined' ? window.location.pathname : '';
  return (
    <>
      <SidebarGroupTitle>Manage</SidebarGroupTitle>
      <ul className="flex flex-col gap-px px-2.5">
        <SidebarMenuItem icon={ShieldCheck} label="Pairing" active={pathname === '/pairing'} onClick={() => router.push('/pairing')} />
      </ul>
      <SidebarDivider />
      <SidebarGroupTitle>Channels</SidebarGroupTitle>
      <ul className="flex flex-col gap-px px-2.5">
        {Object.entries(grouped).map(([type, chs]) => (
          <button key={type} onClick={() => router.push('/channels')}
            className="flex w-full items-center gap-2.5 h-8.5 px-2.5 rounded-md text-2sm font-normal text-muted-foreground hover:text-foreground hover:bg-muted">
            <BrandIcon name={type} size={16} />
            <span className="flex-1 text-left capitalize">{type}</span>
            <span className="rounded-full bg-muted px-1.5 text-2xs">{(chs as any[]).length}</span>
            {(chs as any[]).some((c) => c.status === 'running') && <span className="h-2 w-2 rounded-full bg-emerald-400" />}
          </button>
        ))}
        {channels.length === 0 && <p className="px-2.5 py-4 text-2sm text-muted-foreground">No channels configured</p>}
      </ul>
    </>
  );
}
