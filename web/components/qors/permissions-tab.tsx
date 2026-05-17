'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useCallback } from 'react';
import { cn } from '@/lib/utils';
import { agentPermissions, type PolicyEntry, type PermissionScope } from '@/lib/api';
import {
  Zap,
  Eye,
  ShieldAlert,
  Loader2,
  RotateCcw,
  Check,
  X,
  ChevronDown,
  ChevronRight,
} from 'lucide-react';
import { toast } from 'sonner';

// ─── Tool label + description maps ───────────────────────────────────────────

const TOOL_LABELS: Record<string, string> = {
  exec:           'Shell command execution',
  write_file:     'Write file to disk',
  read_file:      'Read file from disk',
  apply_patch:    'Apply patch to files',
  delete_file:    'Delete a file',
  cron:           'Schedule a recurring task',
  gh_push_file:   'Push file to GitHub',
  gh_open_pr:     'Open a GitHub pull request',
  gh_merge_pr:    'Merge a GitHub pull request',
  gh_create_repo: 'Create a GitHub repository',
  web_search:     'Web search',
  web_fetch:      'Fetch a web page',
  memory_search:  'Search agent memory',
  undo:           'Undo / revert changes',
};

export function toolLabel(name: string) {
  return TOOL_LABELS[name] ?? name.replace(/_/g, ' ');
}

export function describeApproval(
  toolName: string,
  toolArgs: Record<string, unknown> | null | undefined,
): string {
  if (!toolArgs) return toolLabel(toolName);
  switch (toolName) {
    case 'exec':
    case 'apply_patch': {
      const cmd = (toolArgs.command ?? toolArgs.patch ?? '') as string;
      if (cmd) return cmd.length > 80 ? cmd.slice(0, 78) + '…' : cmd;
      break;
    }
    case 'write_file':
    case 'read_file':
    case 'delete_file': {
      const p = toolArgs.path ?? toolArgs.file_path ?? toolArgs.filename;
      if (p) return `${toolLabel(toolName)}: ${p}`;
      break;
    }
    case 'gh_push_file': {
      const f = toolArgs.path ?? toolArgs.file_path;
      if (f) return `Push ${f} to GitHub`;
      break;
    }
    case 'cron': {
      const name = toolArgs.name ?? toolArgs.job_name;
      const sched = toolArgs.schedule ?? toolArgs.cron_expression;
      if (name) return `Schedule "${name}"${sched ? ` (${sched})` : ''}`;
      break;
    }
  }
  return toolLabel(toolName);
}

// ─── Three-tier tool groups ───────────────────────────────────────────────────

type TierID = 'everyday' | 'careful' | 'sensitive';

interface ToolDef {
  name: string;
  label: string;
}

interface Tier {
  id: TierID;
  label: string;
  subtitle: string;
  description: string;
  defaultScope: PermissionScope;
  icon: React.ElementType;
  iconClass: string;
  borderClass: string;
  headerClass: string;
  tools: ToolDef[];
  // Which scope options to offer in this tier's dropdown
  scopeOptions: PermissionScope[];
}

const TIERS: Tier[] = [
  {
    id: 'everyday',
    label: 'Everyday',
    subtitle: 'Auto-approved by default',
    description: 'Read-only research and memory tools — safe to run silently with no interruption.',
    defaultScope: 'auto_approved',
    icon: Zap,
    iconClass: 'text-emerald-500',
    borderClass: 'border-border',
    headerClass: 'bg-muted/40 hover:bg-muted/60',
    scopeOptions: ['auto_approved', 'ask_first', 'blocked'],
    tools: [
      { name: 'read_file',     label: 'Read files from disk' },
      { name: 'web_search',    label: 'Search the web' },
      { name: 'web_fetch',     label: 'Fetch web pages' },
      { name: 'memory_search', label: 'Search agent memory' },
      { name: 'cron',          label: 'Schedule recurring tasks' },
    ],
  },
  {
    id: 'careful',
    label: 'Careful',
    subtitle: 'Ask before running',
    description: 'File writes and patches — can change content but stay on your machine.',
    defaultScope: 'ask_first',
    icon: Eye,
    iconClass: 'text-amber-500',
    borderClass: 'border-amber-500/20',
    headerClass: 'bg-amber-500/5 hover:bg-amber-500/10',
    scopeOptions: ['auto_approved', 'ask_first', 'blocked'],
    tools: [
      { name: 'write_file',  label: 'Write files to disk' },
      { name: 'apply_patch', label: 'Apply patches to files' },
      { name: 'delete_file', label: 'Delete files from disk' },
      { name: 'undo',        label: 'Undo / revert changes' },
    ],
  },
  {
    id: 'sensitive',
    label: 'Sensitive',
    subtitle: 'Review every time',
    description: 'Shell commands and external pushes — can run code, reach the internet, and change shared repos.',
    defaultScope: 'ask_first',
    icon: ShieldAlert,
    iconClass: 'text-destructive',
    borderClass: 'border-destructive/20',
    headerClass: 'bg-destructive/5 hover:bg-destructive/10',
    scopeOptions: ['auto_approved', 'ask_first', 'blocked'],
    tools: [
      { name: 'exec',           label: 'Run shell commands' },
      { name: 'gh_push_file',   label: 'Push files to GitHub' },
      { name: 'gh_open_pr',     label: 'Open pull requests' },
      { name: 'gh_merge_pr',    label: 'Merge pull requests' },
      { name: 'gh_create_repo', label: 'Create GitHub repositories' },
    ],
  },
];

// ─── Scope selector ───────────────────────────────────────────────────────────

const SCOPE_META: Record<PermissionScope, { label: string; dot: string }> = {
  auto_approved: { label: 'Auto-approved', dot: 'bg-emerald-500' },
  ask_first:     { label: 'Ask first',     dot: 'bg-amber-500'   },
  blocked:       { label: 'Blocked',       dot: 'bg-red-500'     },
};

interface ScopeSelectorProps {
  value: PermissionScope;
  options: PermissionScope[];
  onChange: (s: PermissionScope) => void;
  disabled?: boolean;
}

function ScopeSelector({ value, options, onChange, disabled }: ScopeSelectorProps) {
  const meta = SCOPE_META[value];
  return (
    <div className="relative inline-flex items-center gap-1.5 shrink-0">
      <span className={cn('h-2 w-2 rounded-full shrink-0', meta.dot)} />
      <select
        value={value}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value as PermissionScope)}
        className={cn(
          'appearance-none bg-transparent pr-4 text-xs font-medium cursor-pointer',
          'text-foreground disabled:opacity-50 disabled:cursor-not-allowed focus:outline-none',
          value === 'blocked'      && 'text-red-500',
          value === 'ask_first'    && 'text-amber-600 dark:text-amber-400',
          value === 'auto_approved' && 'text-emerald-600 dark:text-emerald-400',
        )}
      >
        {options.map((opt) => (
          <option key={opt} value={opt}>{SCOPE_META[opt].label}</option>
        ))}
      </select>
      <ChevronDown className="pointer-events-none absolute right-0 h-3 w-3 text-muted-foreground" />
    </div>
  );
}

// ─── Approval history types ────────────────────────────────────────────────────

interface ApprovalItem {
  id: string;
  tool: string;
  tool_name?: string;
  reason?: string;
  state?: string;
  status?: string;
  tool_args?: Record<string, unknown> | null;
  created_at: string;
}

// ─── Main component ───────────────────────────────────────────────────────────

interface Props {
  agentId: string;
}

export function PermissionsTab({ agentId }: Props) {
  const [policies, setPolicies] = useState<PolicyEntry[]>([]);
  const [history, setHistory] = useState<ApprovalItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState<string | null>(null);
  const [decidingId, setDecidingId] = useState<string | null>(null);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  const load = useCallback(async () => {
    setLoading(true);
    const pol = await agentPermissions.list(agentId).catch(() => [] as PolicyEntry[]);
    setPolicies(pol);
    setLoading(false);
  }, [agentId]);

  const loadHistory = useCallback(async () => {
    try {
      const token =
        typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') ?? '') : '';
      const result = await fetch(`/api/v1/approvals?agent_id=${agentId}`, {
        headers: { Authorization: `Bearer ${token}` },
      }).then((r) => (r.ok ? r.json() : { approvals: [] }));
      const items: ApprovalItem[] = (result?.approvals ?? [])
        .filter((a: ApprovalItem) => (a.state ?? a.status) !== 'expired')
        .slice(0, 30);
      setHistory(items);
    } catch {
      setHistory([]);
    }
  }, [agentId]);

  useEffect(() => {
    load();
    loadHistory();
  }, [load, loadHistory]);

  const scopeFor = (tool: string, tier: Tier): PermissionScope => {
    const policy = policies.find((p) => p.tool === tool);
    return policy?.scope ?? tier.defaultScope;
  };

  const handleScope = async (tool: string, scope: PermissionScope) => {
    setSaving(tool);
    try {
      await agentPermissions.upsert(agentId, tool, scope);
      setPolicies((prev) => {
        const exists = prev.some((p) => p.tool === tool);
        if (exists) return prev.map((p) => (p.tool === tool ? { ...p, scope } : p));
        return [
          ...prev,
          { id: '', tenant_id: '', user_id: '', agent_id: agentId, tool, scope, created_at: '' },
        ];
      });
    } catch {
      toast.error('Failed to save — please try again');
      load();
    } finally {
      setSaving(null);
    }
  };

  const handleReset = async (tool: string) => {
    setSaving(`reset-${tool}`);
    try {
      await agentPermissions.remove(agentId, tool);
      setPolicies((prev) => prev.filter((p) => p.tool !== tool));
      toast.success(`${toolLabel(tool)} reset to default`);
    } catch {
      toast.error('Failed to reset');
    } finally {
      setSaving(null);
    }
  };

  const handleDecide = async (approvalId: string, decision: 'allow' | 'deny') => {
    setDecidingId(approvalId);
    try {
      const token =
        typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') ?? '') : '';
      await fetch(`/api/v1/approvals/${approvalId}/decide`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ decision }),
      });
      setHistory((prev) =>
        prev.map((a) =>
          a.id === approvalId
            ? { ...a, state: decision === 'allow' ? 'allowed' : 'denied' }
            : a,
        ),
      );
    } catch {
      toast.error('Failed to respond');
    } finally {
      setDecidingId(null);
    }
  };

  const toggleGroup = (id: string) =>
    setCollapsed((prev) => ({ ...prev, [id]: !prev[id] }));

  const pendingHistory = history.filter((a) => (a.state ?? a.status) === 'pending');
  const resolvedHistory = history.filter((a) => (a.state ?? a.status) !== 'pending');

  if (loading) {
    return (
      <div className="flex items-center justify-center h-40 text-muted-foreground text-sm">
        <Loader2 className="h-4 w-4 animate-spin mr-2" /> Loading permissions…
      </div>
    );
  }

  return (
    <div className="max-w-2xl mx-auto space-y-4">

      {/* Tier groups */}
      {TIERS.map((tier) => {
        const Icon = tier.icon;
        const isOpen = !collapsed[tier.id];
        return (
          <div
            key={tier.id}
            className={cn('rounded-xl border overflow-hidden', tier.borderClass)}
          >
            {/* Header */}
            <button
              onClick={() => toggleGroup(tier.id)}
              className={cn(
                'w-full flex items-center justify-between px-4 py-3 text-left transition-colors',
                tier.headerClass,
              )}
            >
              <div className="flex items-center gap-2.5">
                <Icon className={cn('h-5 w-5 shrink-0', tier.iconClass)} strokeWidth={2} />
                <div>
                  <div className="flex items-center gap-2">
                    <p className="text-sm font-semibold leading-none">{tier.label}</p>
                    <span className="text-xs text-muted-foreground leading-none">{tier.subtitle}</span>
                  </div>
                  <p className="text-xs text-muted-foreground mt-0.5 leading-snug">{tier.description}</p>
                </div>
              </div>
              {isOpen ? (
                <ChevronDown className="h-4 w-4 text-muted-foreground shrink-0" strokeWidth={2} />
              ) : (
                <ChevronRight className="h-4 w-4 text-muted-foreground shrink-0" strokeWidth={2} />
              )}
            </button>

            {/* Tool rows */}
            {isOpen && (
              <div className="divide-y divide-border/60">
                {tier.tools.map((tool) => {
                  const current = scopeFor(tool.name, tier);
                  const isSaving = saving === tool.name || saving === `reset-${tool.name}`;
                  const isCustom = policies.some((p) => p.tool === tool.name);
                  return (
                    <div key={tool.name} className="flex items-center gap-3 px-4 py-2.5">
                      <div className="flex-1 min-w-0">
                        <p className="text-sm leading-snug">{tool.label}</p>
                        <p className="text-xs text-muted-foreground font-mono">{tool.name}</p>
                      </div>
                      {isSaving ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground shrink-0" />
                      ) : (
                        <div className="flex items-center gap-2 shrink-0">
                          <ScopeSelector
                            value={current}
                            options={tier.scopeOptions}
                            onChange={(s) => handleScope(tool.name, s)}
                          />
                          <button
                            onClick={() => handleReset(tool.name)}
                            title="Reset to default"
                            disabled={!isCustom}
                            className={cn(
                              'h-6 w-6 flex items-center justify-center rounded transition-colors',
                              isCustom
                                ? 'text-muted-foreground hover:text-foreground hover:bg-muted'
                                : 'text-muted-foreground/20 cursor-default',
                            )}
                          >
                            <RotateCcw className="h-3 w-3" />
                          </button>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}

      {/* Pending approvals */}
      {pendingHistory.length > 0 && (
        <section>
          <h3 className="text-sm font-semibold mb-3">Waiting for Approval</h3>
          <div className="space-y-2">
            {pendingHistory.map((a) => {
              const isDeciding = decidingId === a.id;
              const techName = a.tool_name ?? a.tool;
              return (
                <div
                  key={a.id}
                  className="flex items-start gap-3 rounded-xl border border-amber-500/30 bg-amber-500/5 px-3 py-2.5"
                >
                  <ShieldAlert className="h-4 w-4 text-amber-500 mt-0.5 shrink-0" strokeWidth={2} />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium">
                      {describeApproval(techName, a.tool_args)}
                    </p>
                    <p className="text-xs text-muted-foreground font-mono">{techName}</p>
                    {a.reason && a.reason !== 'command not in allowlist' && (
                      <p className="text-xs text-muted-foreground mt-0.5">{a.reason}</p>
                    )}
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    <button
                      disabled={isDeciding}
                      onClick={() => handleDecide(a.id, 'allow')}
                      className="flex items-center gap-1 rounded-md bg-emerald-500/10 px-2 py-1 text-xs font-medium text-emerald-600 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors"
                    >
                      {isDeciding ? (
                        <Loader2 className="h-3 w-3 animate-spin" />
                      ) : (
                        <Check className="h-3 w-3" />
                      )}
                      Allow
                    </button>
                    <button
                      disabled={isDeciding}
                      onClick={() => handleDecide(a.id, 'deny')}
                      className="flex items-center gap-1 rounded-md bg-destructive/10 px-2 py-1 text-xs font-medium text-destructive hover:bg-destructive/20 disabled:opacity-50 transition-colors"
                    >
                      <X className="h-3 w-3" /> Deny
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      )}

      {/* Approval history */}
      {resolvedHistory.length > 0 && (
        <section>
          <h3 className="text-sm font-semibold mb-3">Approval History</h3>
          <div className="rounded-xl border border-border divide-y divide-border/50 overflow-hidden">
            {resolvedHistory.slice(0, 20).map((a) => {
              const state = a.state ?? a.status ?? 'unknown';
              const techName = a.tool_name ?? a.tool;
              const description = describeApproval(techName, a.tool_args);
              const stateColor =
                state === 'allowed'
                  ? 'text-emerald-500'
                  : state === 'denied'
                    ? 'text-destructive'
                    : 'text-muted-foreground';
              return (
                <div key={a.id} className="flex items-start gap-3 px-3 py-2">
                  <span className={cn('text-xs font-medium shrink-0 w-14 mt-0.5', stateColor)}>
                    {state}
                  </span>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm truncate">{description}</p>
                    <p className="text-xs text-muted-foreground font-mono">{techName}</p>
                  </div>
                  <span className="text-xs text-muted-foreground shrink-0 mt-0.5">
                    {new Date(a.created_at).toLocaleDateString()}
                  </span>
                </div>
              );
            })}
          </div>
        </section>
      )}

      {pendingHistory.length === 0 && resolvedHistory.length === 0 && (
        <p className="text-xs text-muted-foreground">No approval history yet.</p>
      )}
    </div>
  );
}
