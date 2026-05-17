'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { usePathname } from 'next/navigation';
import type { RailSection } from '@/types';

// Ordered most-specific first. useActiveRail picks the first match.
// Rail IDs must match rail.tsx nav items.
const routes: [string, RailSection][] = [
  // Primary surfaces
  ['/qors', 'souls'],
  ['/mail', 'sessions'],
  ['/sessions', 'sessions'],
  ['/social', 'social'],
  ['/drive', 'drive'],
  ['/channels', 'connectors'],
  ['/connections', 'connectors'],
  ['/connectors', 'connectors'],
  ['/pairing', 'connectors'],
  ['/routing', 'connectors'],
  ['/org-chart', 'org-chart'],
  ['/teams', 'org-chart'],
  ['/skills', 'apps'],
  ['/marketplace', 'apps'],
  ['/templates', 'apps'],
  ['/knowledge-graph', 'kg'],
  ['/memories', 'kg'],
  ['/apps', 'apps'],

  // Code hub — all work-management routes highlight the Code rail icon
  ['/code', 'code'],
  ['/github', 'code'],
  ['/tasks', 'code'],
  ['/workflows', 'code'],
  ['/cron', 'code'],
  ['/pipeline', 'code'],
  ['/plans', 'code'],
  ['/approvals', 'code'],
  ['/outbound', 'code'],
  ['/heartbeat', 'code'],
  ['/analytics', 'code'],
  ['/supervisor', 'code'],

  // Labs umbrella
  ['/labs', 'labs'],
  ['/scenarios', 'labs'],
  ['/sandbox', 'labs'],
  ['/research', 'labs'],
  ['/council', 'labs'],

  // Bottom / system
  ['/agents', 'settings'],
  ['/terminal', 'settings'],
  ['/models-hub', 'models'],
  ['/provider-keys', 'models'],
  ['/mcp', 'settings'],
  ['/settings', 'settings'],
  ['/system', 'settings'],
  ['/billing', 'settings'],
  ['/usage', 'settings'],
  ['/audit', 'settings'],
  ['/training', 'settings'],

  // Catch remaining schedule/calendar
  ['/schedule', 'live'],
  ['/calendar', 'live'],
];

export function useActiveRail(): RailSection {
  const pathname = usePathname();
  if (!pathname || pathname === '/') return 'dashboard';
  const match = routes.find(([p]) => pathname === p || pathname.startsWith(p + '/'));
  return match ? match[1] : 'dashboard';
}
