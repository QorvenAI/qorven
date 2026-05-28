'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import {
  Shield, Users, BookOpen, AlertTriangle, Scale, CheckCircle, XCircle, Clock,
  ChevronRight, Layers, Briefcase,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { cn } from '@/lib/utils';
import {
  governanceApi,
  type Designation,
  type SkillFamily,
  type ApprovalRule,
  type ApprovalRequest,
  type PolicyEvent,
  type GovException,
  type ExceptionStats,
} from '@/lib/api-governance';

type Tab = 'designations' | 'approvals' | 'policies' | 'exceptions';

function TabButton({ id, label, icon: Icon, active, onClick }: { id: Tab; label: string; icon: any; active: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'flex items-center gap-2 px-3 py-1.5 rounded-md text-xs font-medium transition-colors',
        active ? 'bg-primary/10 text-primary' : 'text-muted-foreground hover:text-foreground hover:bg-muted/50'
      )}
    >
      <Icon className="h-3.5 w-3.5" />
      {label}
    </button>
  );
}

function StatCard({ icon: Icon, label, value, color }: { icon: any; label: string; value: string; color: string }) {
  return (
    <div className="rounded-lg border border-border bg-card/50 p-4 flex flex-col gap-1">
      <div className="flex items-center gap-2 text-muted-foreground">
        <Icon className={`h-4 w-4 ${color}`} />
        <span className="text-xs">{label}</span>
      </div>
      <span className="text-lg font-semibold text-foreground">{value}</span>
    </div>
  );
}

function Badge({ children, variant = 'default' }: { children: React.ReactNode; variant?: 'default' | 'success' | 'warning' | 'danger' | 'muted' }) {
  const colors = {
    default: 'bg-primary/10 text-primary',
    success: 'bg-emerald-500/10 text-emerald-500',
    warning: 'bg-amber-500/10 text-amber-500',
    danger: 'bg-red-500/10 text-red-500',
    muted: 'bg-muted text-muted-foreground',
  };
  return <span className={cn('inline-flex items-center px-2 py-0.5 rounded text-2xs font-medium', colors[variant])}>{children}</span>;
}

// ─── Designations Tab ───────────────────────────────────────────────────────

function DesignationsTab() {
  const [designations, setDesignations] = useState<Designation[]>([]);
  const [families, setFamilies] = useState<SkillFamily[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<string>('all');

  useEffect(() => {
    Promise.all([governanceApi.listDesignations(), governanceApi.listSkillFamilies()])
      .then(([dRes, fRes]) => {
        setDesignations(dRes.designations || []);
        setFamilies(fRes.skill_families || []);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="flex items-center justify-center p-8 text-muted-foreground text-sm">Loading designations...</div>;

  const depts = [...new Set(designations.map(d => d.department))];
  const filtered = filter === 'all' ? designations : designations.filter(d => d.department === filter);
  const csuite = filtered.filter(d => d.org_layer === 2);
  const workers = filtered.filter(d => d.org_layer === 3);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-4 gap-4">
        <StatCard icon={Users} label="Total Positions" value={designations.length.toString()} color="text-blue-400" />
        <StatCard icon={Briefcase} label="C-Suite (L2)" value={designations.filter(d => d.org_layer === 2).length.toString()} color="text-violet-400" />
        <StatCard icon={Layers} label="Workers (L3)" value={designations.filter(d => d.org_layer === 3).length.toString()} color="text-emerald-400" />
        <StatCard icon={BookOpen} label="Skill Families" value={families.length.toString()} color="text-amber-400" />
      </div>

      {/* Department filter */}
      <div className="flex items-center gap-2 flex-wrap">
        <span className="text-xs text-muted-foreground">Filter:</span>
        <button onClick={() => setFilter('all')} className={cn('px-2 py-1 rounded text-2xs font-medium', filter === 'all' ? 'bg-primary/10 text-primary' : 'text-muted-foreground hover:text-foreground')}>All</button>
        {depts.map(d => (
          <button key={d} onClick={() => setFilter(d)} className={cn('px-2 py-1 rounded text-2xs font-medium capitalize', filter === d ? 'bg-primary/10 text-primary' : 'text-muted-foreground hover:text-foreground')}>{d}</button>
        ))}
      </div>

      {/* C-Suite */}
      {csuite.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">L2 C-Suite ({csuite.length})</h3>
          <div className="rounded-lg border border-border bg-card/50 divide-y divide-border">
            {csuite.map(d => (
              <DesignationRow key={d.id} designation={d} />
            ))}
          </div>
        </div>
      )}

      {/* Workers */}
      {workers.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">L3 Workers ({workers.length})</h3>
          <div className="rounded-lg border border-border bg-card/50 divide-y divide-border">
            {workers.map(d => (
              <DesignationRow key={d.id} designation={d} />
            ))}
          </div>
        </div>
      )}

      {/* Skill Families */}
      {families.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Skill Families ({families.length})</h3>
          <div className="grid grid-cols-2 gap-3">
            {families.map(f => (
              <div key={f.id} className="rounded-lg border border-border bg-card/50 p-3">
                <div className="flex items-center gap-2 mb-1">
                  <div className="w-2 h-2 rounded-full bg-primary/60" />
                  <span className="text-xs font-medium text-foreground capitalize">{f.name}</span>
                </div>
                <p className="text-2xs text-muted-foreground mb-2">{f.description}</p>
                <div className="flex flex-wrap gap-1">
                  {(f.capabilities || []).map(c => <Badge key={c} variant="muted">{c}</Badge>)}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function DesignationRow({ designation: d }: { designation: Designation }) {
  const tierColor = d.model_tier === 'powerful' ? 'text-violet-400' : d.model_tier === 'balanced' ? 'text-blue-400' : 'text-emerald-400';
  return (
    <div className="flex items-center justify-between px-4 py-3 group hover:bg-muted/30">
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-xs font-medium text-foreground truncate">{d.position_name}</span>
          <Badge variant="muted">{d.department}</Badge>
          <span className={cn('text-2xs font-medium', tierColor)}>{d.model_tier}</span>
        </div>
        <p className="text-2xs text-muted-foreground mt-0.5 truncate">{d.nature_of_work}</p>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        {d.can_create_subagents && <Badge variant="default">spawns</Badge>}
        {d.can_approve_actions && <Badge variant="success">approves</Badge>}
        <Badge variant="muted">{d.skill_family}</Badge>
      </div>
    </div>
  );
}

// ─── Approval Matrix Tab ────────────────────────────────────────────────────

function ApprovalsTab() {
  const [rules, setRules] = useState<ApprovalRule[]>([]);
  const [pending, setPending] = useState<ApprovalRequest[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([governanceApi.listApprovalRules(), governanceApi.listPendingApprovals()])
      .then(([rRes, pRes]) => {
        setRules(rRes.rules || []);
        setPending(pRes.requests || []);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const handleDecide = (id: string, status: string) => {
    governanceApi.decideApproval(id, status, '').then(() => {
      setPending(prev => prev.filter(p => p.id !== id));
    });
  };

  if (loading) return <div className="flex items-center justify-center p-8 text-muted-foreground text-sm">Loading approval matrix...</div>;

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-3 gap-4">
        <StatCard icon={Scale} label="Rules Defined" value={rules.length.toString()} color="text-blue-400" />
        <StatCard icon={Clock} label="Pending Approvals" value={pending.length.toString()} color="text-amber-400" />
        <StatCard icon={Shield} label="Human-Required" value={rules.filter(r => r.requires_human).length.toString()} color="text-red-400" />
      </div>

      {/* Pending approvals */}
      {pending.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Pending Decisions ({pending.length})</h3>
          <div className="rounded-lg border border-border bg-card/50 divide-y divide-border">
            {pending.map(p => (
              <div key={p.id} className="flex items-center justify-between px-4 py-3">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-medium text-foreground">{p.action_type}</span>
                    <Badge variant="warning">pending</Badge>
                  </div>
                  <span className="text-2xs text-muted-foreground">{new Date(p.created_at).toLocaleString()}</span>
                </div>
                <div className="flex items-center gap-2">
                  <button onClick={() => handleDecide(p.id, 'approved')} className="px-2 py-1 rounded text-2xs bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20">Approve</button>
                  <button onClick={() => handleDecide(p.id, 'denied')} className="px-2 py-1 rounded text-2xs bg-red-500/10 text-red-500 hover:bg-red-500/20">Deny</button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Rules table */}
      <div className="space-y-2">
        <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Approval Rules ({rules.length})</h3>
        <div className="rounded-lg border border-border bg-card/50 divide-y divide-border">
          {rules.length === 0 ? (
            <div className="p-4 text-xs text-muted-foreground">No rules defined</div>
          ) : (
            rules.map(r => (
              <div key={r.id} className="flex items-center justify-between px-4 py-3">
                <div className="flex items-center gap-3">
                  <div className={cn('w-2 h-2 rounded-full', r.enabled ? 'bg-emerald-400' : 'bg-muted-foreground/30')} />
                  <div>
                    <span className="text-xs font-medium text-foreground">{r.action_type}</span>
                    <div className="flex items-center gap-2 mt-0.5">
                      <span className="text-2xs text-muted-foreground">Approver: <span className="text-foreground">{r.approver_role}</span></span>
                      {r.threshold_usd > 0 && <span className="text-2xs text-muted-foreground">Threshold: ${r.threshold_usd}</span>}
                      {r.auto_approve_below > 0 && <span className="text-2xs text-muted-foreground">Auto-approve below: ${r.auto_approve_below}</span>}
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  {r.requires_human && <Badge variant="danger">human required</Badge>}
                  <Badge variant={r.enabled ? 'success' : 'muted'}>{r.enabled ? 'active' : 'disabled'}</Badge>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}

// ─── Policies Tab ───────────────────────────────────────────────────────────

function PoliciesTab() {
  const [events, setEvents] = useState<PolicyEvent[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    governanceApi.listPolicyEvents()
      .then(res => setEvents(res.events || []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="flex items-center justify-center p-8 text-muted-foreground text-sm">Loading policy events...</div>;

  const actionCounts = events.reduce((acc, e) => {
    acc[e.action_taken] = (acc[e.action_taken] || 0) + 1;
    return acc;
  }, {} as Record<string, number>);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-4 gap-4">
        <StatCard icon={Shield} label="Total Events" value={events.length.toString()} color="text-blue-400" />
        <StatCard icon={XCircle} label="Denied" value={(actionCounts['deny'] || 0).toString()} color="text-red-400" />
        <StatCard icon={AlertTriangle} label="Warnings" value={(actionCounts['warn'] || 0).toString()} color="text-amber-400" />
        <StatCard icon={CheckCircle} label="Logged" value={(actionCounts['log'] || 0).toString()} color="text-emerald-400" />
      </div>

      <div className="space-y-2">
        <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Recent Policy Events</h3>
        <div className="rounded-lg border border-border bg-card/50 divide-y divide-border">
          {events.length === 0 ? (
            <div className="p-4 text-xs text-muted-foreground">No policy events recorded yet</div>
          ) : (
            events.slice(0, 50).map(e => {
              const actionVariant = e.action_taken === 'deny' ? 'danger' : e.action_taken === 'warn' ? 'warning' : e.action_taken === 'log' ? 'muted' : 'default';
              return (
                <div key={e.id} className="flex items-center justify-between px-4 py-2.5">
                  <div className="flex items-center gap-3 min-w-0">
                    <Badge variant={actionVariant}>{e.action_taken}</Badge>
                    <div className="min-w-0">
                      <span className="text-xs text-foreground truncate block">{e.policy_name}</span>
                      <span className="text-2xs text-muted-foreground">{e.trigger_event}</span>
                    </div>
                  </div>
                  <span className="text-2xs text-muted-foreground shrink-0">{new Date(e.created_at).toLocaleString()}</span>
                </div>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
}

// ─── Exceptions Tab ─────────────────────────────────────────────────────────

function ExceptionsTab() {
  const [exceptions, setExceptions] = useState<GovException[]>([]);
  const [stats, setStats] = useState<ExceptionStats | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    governanceApi.listExceptions()
      .then(res => {
        setExceptions(res.exceptions || []);
        setStats(res.stats || null);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const handleResolve = (id: string) => {
    governanceApi.resolveException(id, 'Resolved from dashboard').then(() => {
      setExceptions(prev => prev.filter(e => e.id !== id));
    });
  };

  if (loading) return <div className="flex items-center justify-center p-8 text-muted-foreground text-sm">Loading exceptions...</div>;

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-4 gap-4">
        <StatCard icon={AlertTriangle} label="Total" value={(stats?.total || 0).toString()} color="text-blue-400" />
        <StatCard icon={XCircle} label="Critical" value={(stats?.critical || 0).toString()} color="text-red-400" />
        <StatCard icon={AlertTriangle} label="Warnings" value={(stats?.warning || 0).toString()} color="text-amber-400" />
        <StatCard icon={Clock} label="Unresolved" value={(stats?.unresolved || 0).toString()} color="text-violet-400" />
      </div>

      <div className="space-y-2">
        <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Unresolved Exceptions</h3>
        <div className="rounded-lg border border-border bg-card/50 divide-y divide-border">
          {exceptions.length === 0 ? (
            <div className="p-4 text-xs text-muted-foreground">No unresolved exceptions</div>
          ) : (
            exceptions.map(e => {
              const sevVariant = e.severity === 'critical' ? 'danger' : e.severity === 'warning' ? 'warning' : 'muted';
              return (
                <div key={e.id} className="flex items-center justify-between px-4 py-3">
                  <div className="flex items-center gap-3 min-w-0">
                    <Badge variant={sevVariant}>{e.severity}</Badge>
                    <div className="min-w-0">
                      <span className="text-xs text-foreground truncate block">{e.description}</span>
                      <span className="text-2xs text-muted-foreground">{e.exception_type} &middot; {new Date(e.created_at).toLocaleString()}</span>
                    </div>
                  </div>
                  <button onClick={() => handleResolve(e.id)} className="px-2 py-1 rounded text-2xs bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20 shrink-0">Resolve</button>
                </div>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
}

// ─── Main Page ──────────────────────────────────────────────────────────────

export default function GovernancePage() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const tab = (searchParams.get('tab') as Tab) || 'designations';

  const setTab = (t: Tab) => {
    router.replace(`/governance?tab=${t}`);
  };

  return (
    <div className="flex flex-col h-full">
      <CanvasHeader
        title="Governance"
        description="Designation catalog, approval matrix, policy engine, and exception tracking"
        actions={
          <div className="flex items-center gap-1 bg-muted/30 rounded-lg p-1">
            <TabButton id="designations" label="Designations" icon={Users} active={tab === 'designations'} onClick={() => setTab('designations')} />
            <TabButton id="approvals" label="Approval Matrix" icon={Scale} active={tab === 'approvals'} onClick={() => setTab('approvals')} />
            <TabButton id="policies" label="Policies" icon={Shield} active={tab === 'policies'} onClick={() => setTab('policies')} />
            <TabButton id="exceptions" label="Exceptions" icon={AlertTriangle} active={tab === 'exceptions'} onClick={() => setTab('exceptions')} />
          </div>
        }
      />

      <div className="flex-1 overflow-y-auto px-6 py-5">
        {tab === 'designations' && <DesignationsTab />}
        {tab === 'approvals' && <ApprovalsTab />}
        {tab === 'policies' && <PoliciesTab />}
        {tab === 'exceptions' && <ExceptionsTab />}
      </div>
    </div>
  );
}
