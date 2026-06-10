'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useRouter, usePathname } from 'next/navigation';
import { Brain, Image, Eye, Mic, Globe, GitBranch, BarChart3, Layers, DollarSign, Wallet } from 'lucide-react';
import { SidebarMenuItem, SidebarGroupTitle } from './sidebar-primitives';
import { SidebarLayout } from './sidebar-layout';

const GROUPS = [
  { title: 'Models', items: [
    { href: '/models-hub/generative',   label: 'Generative AI',  icon: Brain },
    { href: '/models-hub/image',        label: 'Images',         icon: Image },
    { href: '/models-hub/video',        label: 'Video',          icon: Eye },
    { href: '/models-hub/tts',          label: 'TTS',            icon: Mic },
    { href: '/models-hub/stt',          label: 'STT',            icon: Mic },
    { href: '/models-hub/search',       label: 'Search',         icon: Globe },
  ] },
  { title: 'Routing', items: [
    { href: '/models-hub/router',       label: 'Model Router',   icon: GitBranch },
    { href: '/models-hub/integrations', label: 'Integrations',   icon: BarChart3 },
    { href: '/models-hub/gateway',      label: 'Gateway',        icon: Layers },
  ] },
  { title: 'Cost', items: [
    { href: '/models-hub/spend',        label: 'Spend',          icon: DollarSign },
    { href: '/budgets',                 label: 'Budgets',        icon: Wallet },
  ] },
] as const;

export function ModelsSidebar() {
  const router   = useRouter();
  const pathname = usePathname();

  return (
    <SidebarLayout
      section3={
        <>
          {GROUPS.map((g) => (
            <div key={g.title}>
              <SidebarGroupTitle>{g.title}</SidebarGroupTitle>
              <ul className="flex flex-col gap-px px-2.5">
                {g.items.map(({ href, label, icon }) => (
                  <SidebarMenuItem
                    key={href}
                    icon={icon}
                    label={label}
                    active={pathname === href || pathname?.startsWith(href + '/')}
                    onClick={() => router.push(href)}
                  />
                ))}
              </ul>
            </div>
          ))}
        </>
      }
    />
  );
}
