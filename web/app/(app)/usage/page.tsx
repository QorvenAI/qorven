'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { Loader2, AlertCircle, DollarSign, Cpu, Zap, TrendingUp } from 'lucide-react';
import { PageShell } from '@/components/layouts/page-shell';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { usage as usageApi, request } from '@/lib/api';
import { cn } from '@/lib/utils';

interface SoulCost {
  id: string;
  name: string;
  cost: number;
  tokens: number;
  calls: number;
}

interface CostData {
  total_cost_this_month: number;
  souls: SoulCost[];
}

interface AgentBudget {
  agent_id: string;
  agent_name: string;
  monthly_usd: number | null;
  spent_this_month: number;
}

function fmt(n: number) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
  return n.toString();
}

function BudgetBar({ spent, limit }: { spent: number; limit: number }) {
  const pct = Math.min((spent / limit) * 100, 100);
  return (
    <div className="flex items-center gap-2 min-w-0">
      <div className="flex-1 h-1.5 rounded-full bg-muted overflow-hidden">
        <div
          className={cn('h-full rounded-full transition-all', pct > 90 ? 'bg-destructive' : pct > 70 ? 'bg-amber-500' : 'bg-primary')}
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className="text-2xs text-muted-foreground shrink-0">${spent.toFixed(2)} / ${limit.toFixed(2)}</span>
    </div>
  );
}

export default function UsagePage() {
  const [data, setData] = useState<CostData | null>(null);
  const [budgets, setBudgets] = useState<AgentBudget[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    Promise.all([
      usageApi.account(),
      request<{ budgets: AgentBudget[] }>('/gateway/budgets').catch(() => ({ budgets: [] })),
    ])
      .then(([usage, bData]) => {
        setData(usage);
        setBudgets(bData?.budgets ?? []);
      })
      .catch(e => setError(e instanceof Error ? e.message : 'Failed to load'))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return (
    <div className="flex items-center justify-center py-20">
      <Loader2 className="w-6 h-6 text-primary animate-spin" />
    </div>
  );

  if (error) return (
    <div className="flex items-center justify-center py-20 text-destructive gap-2">
      <AlertCircle className="w-5 h-5" />{error}
    </div>
  );

  if (!data) return <div className="p-6"><EmptyState {...emptyStates.usage} /></div>;

  const totalTokens = data.souls.reduce((s, a) => s + (a.tokens ?? 0), 0);
  const totalCalls = data.souls.reduce((s, a) => s + a.calls, 0);
  const activeSouls = data.souls.filter(a => a.calls > 0).length;

  const budgetMap = Object.fromEntries((budgets ?? []).map(b => [b.agent_id, b]));

  return (
    <PageShell
      title="Usage & Costs"
      description="Token usage and API costs across all agents this month."
      contentClassName="px-0 py-0 sm:px-0"
    >
      <div className="space-y-6 p-4 lg:p-6 max-w-5xl">
      {/* Stats row */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <StatCard icon={DollarSign} label="Cost this month" value={`$${data.total_cost_this_month.toFixed(4)}`} />
        <StatCard icon={Zap} label="Total tokens" value={fmt(totalTokens)} />
        <StatCard icon={TrendingUp} label="API calls" value={fmt(totalCalls)} />
        <StatCard icon={Cpu} label="Active agents" value={String(activeSouls)} />
      </div>

      {/* Per-agent table */}
      <div className="rounded-xl border border-border overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-card border-b border-border">
            <tr>
              <th className="text-left px-4 py-2.5 text-2xs uppercase tracking-wide text-muted-foreground font-medium">Agent</th>
              <th className="text-right px-4 py-2.5 text-2xs uppercase tracking-wide text-muted-foreground font-medium">Calls</th>
              <th className="text-right px-4 py-2.5 text-2xs uppercase tracking-wide text-muted-foreground font-medium">Tokens</th>
              <th className="text-right px-4 py-2.5 text-2xs uppercase tracking-wide text-muted-foreground font-medium">Cost</th>
              <th className="px-4 py-2.5 text-2xs uppercase tracking-wide text-muted-foreground font-medium">Budget</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {data.souls.map(a => {
              const budget = budgetMap[a.id];
              return (
                <tr key={a.id} className="hover:bg-card/50 transition-colors">
                  <td className="px-4 py-3 font-medium">{a.name || a.id.slice(0, 8)}</td>
                  <td className="px-4 py-3 text-right text-muted-foreground tabular-nums">{fmt(a.calls)}</td>
                  <td className="px-4 py-3 text-right text-muted-foreground tabular-nums">{fmt(a.tokens ?? 0)}</td>
                  <td className="px-4 py-3 text-right font-mono text-primary tabular-nums">${a.cost.toFixed(4)}</td>
                  <td className="px-4 py-3 min-w-[160px]">
                    {budget?.monthly_usd ? (
                      <BudgetBar spent={a.cost} limit={budget.monthly_usd} />
                    ) : (
                      <span className="text-2xs text-muted-foreground/50">No limit</span>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        {!data.souls.length && <p className="text-muted-foreground text-center py-8 text-sm">No usage data yet.</p>}
      </div>
      </div>
    </PageShell>
  );
}

function StatCard({ icon: Icon, label, value }: { icon: typeof DollarSign; label: string; value: string }) {
  return (
    <div className="rounded-xl border border-border bg-card/40 p-4 flex items-center gap-3">
      <div className="h-9 w-9 rounded-lg bg-primary/10 flex items-center justify-center shrink-0">
        <Icon className="h-4 w-4 text-primary" />
      </div>
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground truncate">{label}</p>
        <p className="text-lg font-semibold tabular-nums">{value}</p>
      </div>
    </div>
  );
}
