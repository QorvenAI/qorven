'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect } from 'react';
import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';
import { useStore } from '@/store';
import { agents as agentsApi } from '@/lib/api';
import { ActivityTab } from '@/components/tabs/activity-tab';
import { MapPin, Briefcase, Cpu, Shield, Wallet, Sparkles, Activity, User, Loader2, Save } from 'lucide-react';
import { toast } from 'sonner';
import type { Soul } from '@/types';
import { modelDisplayName } from '@/lib/model-names';

const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

interface Props { soul: Soul }

const subPages = [
  { id: 'profile',     label: 'Profile',       icon: User },
  { id: 'budget',      label: 'Budget',         icon: Wallet },
  { id: 'permissions', label: 'Permissions',    icon: Shield },
  { id: 'bundles',     label: 'Instructions',   icon: Sparkles },
  { id: 'activity',    label: 'Activity',       icon: Activity },
];

export function SoulSettingsPage({ soul }: Props) {
  const [activeSub, setActiveSub] = useState('profile');
  const soulState = useStore((s) => s.soulStates[soul.id]);
  const activity = soulState?.activity ?? 'idle';

  return (
    <div className="max-w-4xl mx-auto space-y-0">
      {/* Hero card — profile header */}
      <div className="rounded-xl border border-border bg-card overflow-hidden">
        {/* Banner */}
        <div className="h-28 bg-gradient-to-r from-primary/20 via-primary/60/20 to-purple-600/20" />

        {/* Profile info */}
        <div className="px-6 pb-4 -mt-10">
          <div className="flex items-end gap-4">
            <div className={cn('flex h-20 w-20 items-center justify-center rounded-xl bg-gradient-to-br text-2xl font-semibold text-white border-4 border-background shadow-lg', soulGradient(soul.display_name))}>
              {soul.display_name.charAt(0)}
            </div>
            <div className="flex-1 pb-1">
              <div className="flex items-center gap-3">
                <h1 className="text-xl font-semibold">{soul.display_name}</h1>
                <span className={cn('rounded-full px-2 py-0.5 text-2xs font-medium',
                  activity === 'idle' ? 'bg-emerald-400/10 text-emerald-400' :
                  activity === 'thinking' ? 'bg-amber-400/10 text-amber-400' : 'bg-blue-400/10 text-blue-400')}>
                  {activity === 'idle' ? '🟢 Online' : activity === 'thinking' ? '●●● Thinking' : '▶ Running'}
                </span>
              </div>
              <div className="flex items-center gap-4 mt-1 text-xs text-muted-foreground">
                {soul.role && <span className="flex items-center gap-1"><Briefcase className="h-3 w-3" />{soul.role}</span>}
                {soul.title && <span className="flex items-center gap-1"><MapPin className="h-3 w-3" />{soul.title}</span>}
                <span className="flex items-center gap-1"><Cpu className="h-3 w-3" />{modelDisplayName(soul.model)}</span>
              </div>
            </div>
            <div className="flex gap-6 pb-1">
              <Stat label="Tokens" value={soul.credit_used_cents > 0 ? `$${(soul.credit_used_cents / 100).toFixed(2)}` : '0'} />
              <Stat label="Skills" value={String(soul.skills?.length ?? 0)} />
              <Stat label="Memory" value={soul.memory_enabled ? 'On' : 'Off'} />
            </div>
          </div>
        </div>

        {/* Horizontal sub-nav */}
        <div className="border-t border-border px-6">
          <nav className="flex gap-0.5 -mb-px">
            {subPages.map(({ id, label, icon: Icon }) => (
              <button key={id} onClick={() => setActiveSub(id)}
                className={cn('flex items-center gap-1.5 px-4 py-3 text-xs font-medium border-b-2 transition-colors',
                  activeSub === id ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground hover:border-border')}>
                <Icon className="h-3.5 w-3.5" />{label}
              </button>
            ))}
          </nav>
        </div>
      </div>

      {/* Sub-page content */}
      <div className="pt-5">
        {activeSub === 'profile' && <ProfileSubPage soul={soul} />}
        {activeSub === 'budget' && <BudgetSubPage soul={soul} />}
          {activeSub === 'permissions' && (
            <div className="space-y-4">
              <p className="text-sm font-medium">Role & Permissions</p>
              <div className="rounded-xl border border-border p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs text-muted-foreground">Role</span>
                  <select defaultValue={soul.role || 'specialist'} onChange={e => agentsApi.update(soul.id, { role: e.target.value })}
                    className="rounded border border-input bg-transparent px-2 py-1 text-xs">
                    <option value="chief">Prime (Full Access)</option>
                    <option value="director">Lead (Department)</option>
                    <option value="specialist">Qor (Specialist)</option>
                  </select>
                </div>
                <p className="text-2xs text-muted-foreground">Prime: all tools + delegation. Lead: department tools. Qor: assigned tools only.</p>
              </div>
            </div>
          )}
          {activeSub === 'bundles' && (
            <div className="space-y-4">
              <p className="text-sm font-medium">Instruction Bundles</p>
              <p className="text-xs text-muted-foreground">Custom instructions injected into this Qor's system prompt.</p>
              {['identity', 'tools', 'soul'].map(type => (
                <div key={type} className="rounded-xl border border-border p-4">
                  <p className="text-xs font-medium mb-2 capitalize">{type}.md</p>
                  <textarea defaultValue="" placeholder={`${type} instructions for this Qor...`}
                    className="w-full rounded border border-input bg-transparent px-3 py-2 text-xs font-mono h-24 resize-y" />
                  <button className="mt-2 rounded bg-primary px-3 py-1 text-xs text-primary-foreground cursor-pointer">Save</button>
                </div>
              ))}
            </div>
          )}

        {activeSub === 'activity' && <ActivityTab agentId={soul.id} />}
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="text-center">
      <p className="text-sm font-semibold">{value}</p>
      <p className="text-2xs text-muted-foreground">{label}</p>
    </div>
  );
}

// ─── Profile Sub-Page ───

function ProfileSubPage({ soul }: { soul: Soul }) {
  const isPrime = soul.role === 'prime' || soul.agent_key === '__prime__';
  const [displayName, setDisplayName] = useState(soul.display_name);
  const [role, setRole] = useState(soul.role || 'worker');
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    setSaving(true);
    try {
      await agentsApi.update(soul.id, { display_name: displayName, role: isPrime ? soul.role : role });
      toast.success('Profile saved');
    } catch {
      toast.error('Failed to save profile');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-5 max-w-2xl">
      <Card title="Identity">
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="text-2xs font-medium text-muted-foreground uppercase tracking-wider">Display Name</label>
            <input value={displayName} onChange={(e) => setDisplayName(e.target.value)}
              className="qr-input" />
          </div>
          <div>
            <label className="text-2xs font-medium text-muted-foreground uppercase tracking-wider">Role</label>
            {isPrime ? (
              <div className="mt-1 flex items-center gap-2 rounded-lg border border-border bg-muted/10 px-3 py-2 text-sm text-muted-foreground">
                Prime
                <span className="ml-auto text-2xs bg-primary/10 text-primary px-1.5 py-0.5 rounded">Locked</span>
              </div>
            ) : (
              <select value={role} onChange={(e) => setRole(e.target.value)}
                className="qr-select">
                <option value="supervisor">Supervisor</option>
                <option value="worker">Worker</option>
                <option value="researcher">Researcher</option>
                <option value="developer">Developer</option>
                <option value="writer">Writer</option>
              </select>
            )}
          </div>
          <div>
            <label className="text-2xs font-medium text-muted-foreground uppercase tracking-wider">Agent Key</label>
            <p className="mt-1 text-sm text-muted-foreground font-mono">{soul.agent_key}</p>
          </div>
          <div>
            <label className="text-2xs font-medium text-muted-foreground uppercase tracking-wider">Model</label>
            <p className="mt-1 text-sm text-muted-foreground">{modelDisplayName(soul.model)}</p>
          </div>
        </div>
        <button onClick={handleSave} disabled={saving}
          className="mt-4 flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
          {saving ? 'Saving…' : 'Save Profile'}
        </button>
      </Card>

      {soul.system_prompt && (
        <Card title="System Prompt">
          <pre className="rounded-lg bg-muted px-3 py-2 text-xs max-h-60 overflow-y-auto whitespace-pre-wrap font-mono text-muted-foreground">{soul.system_prompt}</pre>
        </Card>
      )}
      <Card title="Skills">
        <div className="flex flex-wrap gap-1.5">
          {(soul.skills?.length ?? 0) === 0 ? <p className="text-xs text-muted-foreground">No skills installed</p> :
            soul.skills?.map((s) => <span key={s} className="rounded-md bg-primary/10 text-primary px-2.5 py-1 text-xs">{s}</span>)}
        </div>
      </Card>
    </div>
  );
}

// ─── Budget Sub-Page ───

function BudgetSubPage({ soul }: { soul: Soul }) {
  const [usageData, setUsageData] = useState<any>(null);
  const [budgetInput, setBudgetInput] = useState(String(soul.credit_budget_cents / 100));
  useEffect(() => {
    fetch(`/api/v1/usage/soul/${soul.id}`, { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json()).then(setUsageData).catch(() => {});
  }, [soul.id]);

  const saveBudget = () => {
    const cents = Math.round(parseFloat(budgetInput) * 100);
    fetch(`/api/v1/agents/${soul.id}/budget`, {
      method: "PUT", headers: { "Content-Type": "application/json", Authorization: `Bearer ${getToken()}` },
      body: JSON.stringify({ budget_cents: cents }),
    });
  };

  const month = usageData?.this_month || {};
  const allTime = usageData?.all_time || {};
  const topModels = usageData?.top_models || [];

  return (
    <div className="space-y-5 max-w-2xl">
      <Card title="Monthly Budget">
        <div className="flex items-center gap-3">
          <span className="text-sm text-muted-foreground">$</span>
          <input type="number" step="0.01" min="0" value={budgetInput} onChange={e => setBudgetInput(e.target.value)}
            className="w-32 rounded-lg border border-border bg-background px-3 py-2 text-sm" />
          <button onClick={saveBudget} className="rounded-lg bg-primary px-4 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 cursor-pointer">Set Budget</button>
          <span className="text-2xs text-muted-foreground">0 = unlimited</span>
        </div>
      </Card>
      <Card title="Usage This Month">
        <div className="grid grid-cols-3 gap-4">
          <div className="rounded-lg border border-border p-3 text-center">
            <p className="text-lg font-semibold">${(month.cost || 0).toFixed(4)}</p>
            <p className="text-2xs text-muted-foreground">Cost</p>
          </div>
          <div className="rounded-lg border border-border p-3 text-center">
            <p className="text-lg font-semibold">{(month.calls || 0).toLocaleString()}</p>
            <p className="text-2xs text-muted-foreground">Calls</p>
          </div>
          <div className="rounded-lg border border-border p-3 text-center">
            <p className="text-lg font-semibold">{((month.tokens || 0) / 1000).toFixed(1)}K</p>
            <p className="text-2xs text-muted-foreground">Tokens</p>
          </div>
        </div>
      </Card>
      <Card title="All Time">
        <div className="grid grid-cols-3 gap-4">
          <div className="rounded-lg border border-border p-3 text-center">
            <p className="text-lg font-semibold">${(allTime.cost || 0).toFixed(4)}</p>
            <p className="text-2xs text-muted-foreground">Total Cost</p>
          </div>
          <div className="rounded-lg border border-border p-3 text-center">
            <p className="text-lg font-semibold">{(allTime.calls || 0).toLocaleString()}</p>
            <p className="text-2xs text-muted-foreground">Total Calls</p>
          </div>
          <div className="rounded-lg border border-border p-3 text-center">
            <p className="text-lg font-semibold">{((allTime.tokens || 0) / 1000).toFixed(1)}K</p>
            <p className="text-2xs text-muted-foreground">Total Tokens</p>
          </div>
        </div>
      </Card>
      {topModels.length > 0 && (
        <Card title="Top Models Used">
          <div className="space-y-2">
            {topModels.map((m: any) => {
              const total = topModels.reduce((s: number, x: any) => s + (x.cost || 0), 0);
              const pct = total > 0 ? (m.cost / total) * 100 : 0;
              return (
                <div key={m.model} className="flex items-center gap-3">
                  <span className="text-xs w-32 truncate">{modelDisplayName(m.model)}</span>
                  <div className="flex-1 h-2 rounded-full bg-muted overflow-hidden">
                    <div className="h-full rounded-full bg-primary" style={{ width: `${pct}%` }} />
                  </div>
                  <span className="text-2xs text-muted-foreground w-16 text-right">${(m.cost || 0).toFixed(4)}</span>
                </div>
              );
            })}
          </div>
        </Card>
      )}
    </div>
  );
}

// ─── Shared Components ───

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-xl border border-border bg-card">
      <div className="px-5 py-3.5 border-b border-border">
        <h3 className="text-sm font-semibold">{title}</h3>
      </div>
      <div className="px-5 py-4">{children}</div>
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between py-1.5 border-b border-border last:border-0">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="text-xs font-medium">{value}</span>
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <label className="text-2xs font-medium text-muted-foreground uppercase tracking-wider">{label}</label>
      <p className="mt-0.5 text-sm">{value}</p>
    </div>
  );
}
