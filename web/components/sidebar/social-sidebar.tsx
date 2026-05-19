'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { apiBase } from '@/lib/api-url';
import { Megaphone, Bell, Clock, CheckCircle2, FileEdit, Users, Zap } from 'lucide-react';
import { SidebarMenuItem } from './sidebar-primitives';
import { SidebarLayout } from './sidebar-layout';

export function SocialSidebar() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const tab = searchParams.get('tab') ?? 'compose';
  const [stats, setStats] = useState<{ scheduled: number; published: number; drafts: number } | null>(null);

  useEffect(() => {
    const token = typeof window !== 'undefined' ? localStorage.getItem('qorven_token') || '' : '';
    fetch(`${apiBase()}/social/calendar`, {
      headers: { Authorization: `Bearer ${token}` },
    }).then(r => r.json()).then(d => setStats(d?.stats)).catch(() => {});
  }, []);

  const views = [
    { label: 'Compose', sub: '/social?tab=compose', icon: Megaphone },
    { label: 'Calendar', sub: '/social?tab=calendar', icon: Bell },
    { label: 'Scheduled', sub: '/social?tab=scheduled', icon: Clock },
    { label: 'Published', sub: '/social?tab=published', icon: CheckCircle2 },
    { label: 'Drafts', sub: '/social?tab=drafts', icon: FileEdit },
    { label: 'Accounts', sub: '/social?tab=accounts', icon: Users },
    { label: 'AutoPost', sub: '/social?tab=autopost', icon: Zap },
  ];

  return (
    <SidebarLayout
      section3={
        <ul className="flex flex-col gap-px px-2.5">
          {views.map(v => {
            const id = v.sub.split('?tab=')[1] ?? 'compose';
            return (
              <SidebarMenuItem
                key={v.label}
                icon={v.icon}
                label={v.label}
                active={tab === id}
                badge={v.label === 'Scheduled' && stats?.scheduled ? String(stats.scheduled) : undefined}
                badgeColor="bg-primary/10 text-primary"
                onClick={() => router.push(v.sub)}
              />
            );
          })}
        </ul>
      }
    />
  );
}
