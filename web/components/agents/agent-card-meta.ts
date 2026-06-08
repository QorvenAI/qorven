// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
//
// Shared presentation helpers for agent profile cards (used by /qors and the
// org chart). Keeps the department mapping and tagline extraction in one place
// so both surfaces render identical card content.

import type { Soul } from '@/types';

// org_role → human department label shown as the single function chip.
const ROLE_DEPARTMENT: Record<string, string> = {
  coo:  'Operations',
  cto:  'Engineering',
  cmo:  'Marketing',
  cso:  'Sales',
  cco:  'Customer',
  chro: 'People',
  cfo:  'Finance',
  cko:  'Knowledge',
  caio: 'AI Ops',
  ciso: 'Security',
};

// Worker (L3) role archetype → department, when no org_role is set.
const ARCHETYPE_DEPARTMENT: Record<string, string> = {
  code:       'Engineering',
  coder:      'Engineering',
  architect:  'Engineering',
  reviewer:   'Engineering',
  devops:     'Engineering',
  qa:         'Engineering',
  marketer:   'Marketing',
  social:     'Marketing',
  writer:     'Content',
  researcher: 'Research',
  analyst:    'Analytics',
  sales:      'Sales',
  support:    'Customer',
  legal:      'Compliance',
  designer:   'Design',
  product:    'Product',
};

/** The single department/function tag for an agent's card. Empty string = show none. */
export function agentDepartment(soul: Pick<Soul, 'org_role' | 'role'>): string {
  if (soul.org_role && ROLE_DEPARTMENT[soul.org_role]) return ROLE_DEPARTMENT[soul.org_role]!;
  if (soul.role && ARCHETYPE_DEPARTMENT[soul.role]) return ARCHETYPE_DEPARTMENT[soul.role]!;
  return '';
}

/** Number of agents that report to the given manager (direct reports). */
export function directReportCount(managerId: string, all: Pick<Soul, 'manager_id'>[]): number {
  return all.filter((a) => a.manager_id === managerId).length;
}

// Capability flag → human-readable skill chip label.
const CAPABILITY_LABELS: { test: (s: Soul) => boolean; label: string }[] = [
  { test: (s) => !!s.web_search_enabled,            label: 'Web Search' },
  { test: (s) => !!s.memory_enabled,                label: 'Memory' },
  { test: (s) => !!s.can_delegate,                  label: 'Delegation' },
  { test: (s) => s.tool_profile === 'full',         label: 'Full Tools' },
];

// skills[] slug → friendly label.
const SKILL_LABELS: Record<string, string> = {
  sdk:        'SDK',
  coding:     'Coding',
  delegation: 'Delegation',
  dashboard:  'Dashboard',
  research:   'Research',
  content:    'Content',
  sales:      'Sales',
  support:    'Support',
  analytics:  'Analytics',
};

/** Human-readable capability chips for an agent — combines skills[] + capability flags, deduped. */
export function agentCapabilities(soul: Soul): string[] {
  const out = new Set<string>();
  for (const s of soul.skills ?? []) {
    out.add(SKILL_LABELS[s] ?? (s.charAt(0).toUpperCase() + s.slice(1)));
  }
  for (const c of CAPABILITY_LABELS) {
    if (c.test(soul)) out.add(c.label);
  }
  return Array.from(out);
}

/** A one-line tagline from the agent's system prompt — the first meaningful sentence. */
export function agentTagline(soul: Pick<Soul, 'system_prompt' | 'title'>): string {
  const sp = (soul.system_prompt ?? '').trim();
  if (sp) {
    // First non-empty line, strip leading "You are X — " / "You are X, " preambles.
    const firstLine = sp.split('\n').map(l => l.trim()).find(Boolean) ?? '';
    const cleaned = firstLine
      .replace(/^you are\b.*?[—,-]\s*/i, '')  // "You are the Prime — ..." → "..."
      .replace(/^you are\b\s*/i, '')           // bare "You are ..." → "..."
      .trim();
    const sentence = (cleaned || firstLine).split(/(?<=[.!?])\s/)[0] ?? '';
    if (sentence) {
      const out = sentence.charAt(0).toUpperCase() + sentence.slice(1);
      return out.length > 90 ? out.slice(0, 88).trimEnd() + '…' : out;
    }
  }
  return soul.title || '';
}
