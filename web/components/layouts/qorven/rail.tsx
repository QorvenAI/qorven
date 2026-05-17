'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { cn } from '@/lib/utils';
import type { RailSection } from '@/types';
import {
  LayoutDashboard, MessageSquare, Code, Mail,
  Megaphone, HardDrive, Link2,
  Share2, GitFork, Beaker,
  Settings, Brain, Package,
} from 'lucide-react';
import { useActiveRail } from '@/hooks/use-active-rail';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/qor/tooltip';

type NavItem = { id: RailSection; icon: typeof MessageSquare; label: string; href: string };

const primary: NavItem[] = [
  { id: 'dashboard',  icon: LayoutDashboard, label: 'Dashboard',  href: '/' },
  { id: 'souls',      icon: MessageSquare,   label: 'Chat',        href: '/qors' },
  { id: 'code',       icon: Code,            label: 'Code',        href: '/code' },
  { id: 'sessions',   icon: Mail,            label: 'Email',       href: '/mail' },
  { id: 'social',     icon: Megaphone,       label: 'Social',      href: '/social' },
  { id: 'drive',      icon: HardDrive,       label: 'Drive',       href: '/drive' },
  { id: 'connectors', icon: Link2,           label: 'Channels',    href: '/channels' },
  { id: 'org-chart',  icon: GitFork,         label: 'Org Chart',   href: '/org-chart' },
  { id: 'kg',         icon: Share2,          label: 'Knowledge',   href: '/knowledge-graph' },
  { id: 'apps',       icon: Package,         label: 'Apps',        href: '/apps' },
];

const bottom: NavItem[] = [
  { id: 'labs',     icon: Beaker,   label: 'Labs',     href: '/labs' },
  { id: 'models',   icon: Brain,    label: 'Models',   href: '/models-hub' },
  { id: 'settings', icon: Settings, label: 'Settings', href: '/settings' },
];

export const SIDEBAR_SECTIONS = new Set<RailSection>([
  'home', 'dashboard', 'souls', 'sessions', 'live', 'social',
  'connectors', 'org-chart', 'kg',
  'labs', 'models', 'settings',
]);

export function Rail() {
  const router = useRouter();
  const activeRail = useActiveRail();

  const renderItem = ({ id, icon: Icon, label, href }: NavItem) => (
    <Tooltip key={id}>
      <TooltipTrigger asChild>
        <button
          onClick={() => router.push(href)}
          className={cn(
            'flex h-9 w-9 items-center justify-center rounded-md transition-colors',
            id === activeRail
              ? 'bg-primary text-primary-foreground'
              : 'text-muted-foreground hover:text-foreground hover:bg-accent',
          )}
        >
          <Icon className="h-[18px] w-[18px]" strokeWidth={2} />
        </button>
      </TooltipTrigger>
      <TooltipContent side="right" sideOffset={8}>{label}</TooltipContent>
    </Tooltip>
  );

  return (
    <TooltipProvider delayDuration={200}>
      <div className="rail fixed top-0 bottom-0 left-0 z-30 flex flex-col items-center bg-muted border-e border-border">
        {/* Logo — links to dashboard */}
        <div className="flex h-[var(--header-height)] w-full items-center justify-center shrink-0">
          <Link href="/" title="Dashboard" className="flex items-center justify-center rounded-md hover:opacity-80 transition-opacity">
            <img src="/logo/qorven-mark.svg" alt="Qorven" className="h-[28px]" />
          </Link>
        </div>

        {/* Primary nav */}
        <nav className="flex flex-1 flex-col items-center gap-1 py-2 overflow-y-auto scrollbar-none">
          {primary.map(renderItem)}
        </nav>

        {/* Bottom pinned */}
        <div className="flex flex-col items-center gap-1 py-2 shrink-0 border-t border-border">
          {bottom.map(renderItem)}
        </div>
      </div>
    </TooltipProvider>
  );
}
