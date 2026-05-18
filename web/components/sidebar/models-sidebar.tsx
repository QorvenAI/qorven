'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useRouter, usePathname } from 'next/navigation';
import { Brain, Image, Eye, Mic, Globe, GitBranch, BarChart3 } from 'lucide-react';
import { SidebarMenuItem } from './sidebar-primitives';

const ITEMS = [
  { href: '/models-hub/generative',   label: 'Generative AI',  icon: Brain },
  { href: '/models-hub/image',        label: 'Images',         icon: Image },
  { href: '/models-hub/video',        label: 'Video',          icon: Eye },
  { href: '/models-hub/tts',          label: 'TTS',            icon: Mic },
  { href: '/models-hub/stt',          label: 'STT',            icon: Mic },
  { href: '/models-hub/search',       label: 'Search',         icon: Globe },
  { href: '/models-hub/router',       label: 'Model Router',   icon: GitBranch },
  { href: '/models-hub/integrations', label: 'Integrations',   icon: BarChart3 },
] as const;

export function ModelsSidebar() {
  const router   = useRouter();
  const pathname = usePathname();

  return (
    <ul className="flex flex-col gap-px px-2.5 pt-3">
      {ITEMS.map(({ href, label, icon }) => (
        <SidebarMenuItem
          key={href}
          icon={icon}
          label={label}
          active={pathname === href || pathname?.startsWith(href + '/')}
          onClick={() => router.push(href)}
        />
      ))}
    </ul>
  );
}
