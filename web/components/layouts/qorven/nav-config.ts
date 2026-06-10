// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Single source of truth for the grouped sidebar navigation. Each group is a
// collapsible accordion section; each item is a nav link. Edit membership here.

import {
  LayoutDashboard, MessageSquare, Code, Package,
  Mail, Megaphone, Link2, Send, Rss,
  HardDrive, Share2, Brain as BrainIcon, FlaskConical,
  GitFork, Users, Bot, Target, CheckSquare, Wallet, ShieldCheck,
  Workflow, GitBranch, ListTodo, Boxes, Cable, Plug, Sparkles, Wrench,
  BarChart3, ScrollText, Radio, Activity, Gauge, History,
  Cpu, KeyRound, Beaker, Settings,
  type LucideIcon,
} from 'lucide-react';

export type NavMenuItem = { title: string; path: string; icon: LucideIcon; badge?: string };
export type NavMenuGroup = { id: string; title: string; children: NavMenuItem[] };

export const NAV_GROUPS: NavMenuGroup[] = [
  {
    id: 'workspace', title: 'Workspace', children: [
      { title: 'Dashboard', path: '/', icon: LayoutDashboard },
      { title: 'Chat', path: '/qors', icon: MessageSquare },
      { title: 'Code', path: '/code', icon: Code },
      { title: 'Apps', path: '/apps', icon: Package },
    ],
  },
  {
    id: 'communicate', title: 'Communicate', children: [
      { title: 'Email', path: '/mail', icon: Mail },
      { title: 'Social', path: '/social', icon: Megaphone },
      { title: 'Channels', path: '/channels', icon: Link2 },
      { title: 'Outbound', path: '/outbound', icon: Send },
      { title: 'Content Feed', path: '/content-feed', icon: Rss },
    ],
  },
  {
    id: 'knowledge', title: 'Knowledge & Data', children: [
      { title: 'Drive', path: '/drive', icon: HardDrive },
      { title: 'Knowledge Graph', path: '/knowledge-graph', icon: Share2 },
      { title: 'Memories', path: '/memories', icon: BrainIcon },
      { title: 'Research', path: '/research', icon: FlaskConical },
    ],
  },
  {
    id: 'organization', title: 'Organization', children: [
      { title: 'Org Chart', path: '/org-chart', icon: GitFork },
      { title: 'Teams', path: '/teams', icon: Users },
      { title: 'Agents', path: '/agents', icon: Bot },
      { title: 'Goals', path: '/goals', icon: Target },
      { title: 'Approvals', path: '/approvals', icon: CheckSquare },
      { title: 'Budgets', path: '/budgets', icon: Wallet },
      { title: 'Governance', path: '/governance', icon: ShieldCheck },
    ],
  },
  {
    id: 'build', title: 'Build & Automate', children: [
      { title: 'Workflows', path: '/workflows', icon: Workflow },
      { title: 'Pipeline', path: '/pipeline', icon: GitBranch },
      { title: 'Tasks', path: '/tasks', icon: ListTodo },
      { title: 'Scenarios', path: '/scenarios', icon: Boxes },
      { title: 'Connectors', path: '/connectors', icon: Plug },
      { title: 'MCP', path: '/mcp', icon: Cable },
      { title: 'Skills', path: '/skills', icon: Sparkles },
      { title: 'Tools', path: '/tools', icon: Wrench },
    ],
  },
  {
    id: 'observe', title: 'Observe', children: [
      { title: 'Analytics', path: '/analytics', icon: BarChart3 },
      { title: 'Audit', path: '/audit', icon: ScrollText },
      { title: 'Sessions', path: '/sessions', icon: Radio },
      { title: 'Traces', path: '/traces', icon: Activity },
      { title: 'Usage', path: '/usage', icon: Gauge },
      { title: 'Heartbeat', path: '/heartbeat', icon: History },
      { title: 'Quality', path: '/quality', icon: CheckSquare },
    ],
  },
  {
    id: 'system', title: 'System', children: [
      { title: 'Models', path: '/models-hub', icon: Cpu },
      { title: 'Provider Keys', path: '/provider-keys', icon: KeyRound },
      { title: 'Labs', path: '/labs', icon: Beaker },
      { title: 'Settings', path: '/settings', icon: Settings },
    ],
  },
];
