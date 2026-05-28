'use client';

import { useEffect, useState } from 'react';
import { request } from '@/lib/api-core';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { DollarSign, TrendingUp, AlertTriangle, BarChart3 } from 'lucide-react';

interface AgentCost {
  agent_id: string;
  agent_key: string;
  cost_usd: number;
  tokens_in: number;
  tokens_out: number;
}

interface DailyCost {
  date: string;
  cost_usd: number;
}

interface Anomaly {
  agent_id: string;
  agent_key: string;
  today_cost_usd: number;
  avg_daily_cost_usd: number;
  ratio: number;
}

interface BreakdownData {
  total_cost_usd: number;
  by_agent: AgentCost[];
  daily_trend: DailyCost[];
  avg_daily_usd: number;
  days_remaining: number;
  projected_month_usd: number;
}

function StatCard({ icon: Icon, label, value, sub }: { icon: any; label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-lg border border-border bg-card/50 p-4 flex flex-col gap-1">
      <div className="flex items-center gap-2 text-muted-foreground">
        <Icon className="h-4 w-4" />
        <span className="text-xs">{label}</span>
      </div>
      <span className="text-lg font-semibold text-foreground">{value}</span>
      {sub && <span className="text-2xs text-muted-foreground">{sub}</span>}
    </div>
  );
}

function CostBar({ agent, maxCost }: { agent: AgentCost; maxCost: number }) {
  const pct = maxCost > 0 ? (agent.cost_usd / maxCost) * 100 : 0;
  return (
    <div className="flex items-center gap-3 py-1.5">
      <span className="text-xs text-foreground w-28 truncate font-medium">{agent.agent_key}</span>
      <div className="flex-1 h-2 rounded-full bg-muted/30 overflow-hidden">
        <div className="h-full rounded-full bg-emerald-500/70" style={{ width: `${Math.min(pct, 100)}%` }} />
      </div>
      <span className="text-xs text-muted-foreground w-16 text-right">${agent.cost_usd.toFixed(2)}</span>
    </div>
  );
}

export default function FinancePage() {
  const [data, setData] = useState<BreakdownData | null>(null);
  const [anomalies, setAnomalies] = useState<Anomaly[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      request<BreakdownData>('/billing/breakdown').catch(() => null),
      request<{ anomalies: Anomaly[] }>('/billing/anomalies').catch(() => ({ anomalies: [] })),
    ]).then(([bd, an]) => {
      setData(bd);
      setAnomalies(an?.anomalies || []);
    }).finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div className="flex flex-col h-full">
        <CanvasHeader title="Finance" description="CFO-grade cost visibility across the agent workforce" />
        <div className="flex-1 flex items-center justify-center text-muted-foreground text-sm">Loading...</div>
      </div>
    );
  }

  const totalCost = data?.total_cost_usd || 0;
  const projected = data?.projected_month_usd || 0;
  const avgDaily = data?.avg_daily_usd || 0;
  const agents = data?.by_agent || [];
  const daily = data?.daily_trend || [];
  const maxAgentCost = agents.length > 0 ? (agents[0]?.cost_usd ?? 1) : 1;

  return (
    <div className="flex flex-col h-full">
      <CanvasHeader title="Finance" description="CFO-grade cost visibility across the agent workforce" />

      <div className="flex-1 overflow-y-auto px-6 py-5 space-y-6">
        {/* Summary stats */}
        <div className="grid grid-cols-4 gap-4">
          <StatCard icon={DollarSign} label="Month-to-Date" value={`$${totalCost.toFixed(2)}`} />
          <StatCard icon={TrendingUp} label="Projected Month" value={`$${projected.toFixed(2)}`} sub={`${data?.days_remaining || 0} days remaining`} />
          <StatCard icon={BarChart3} label="Avg Daily" value={`$${avgDaily.toFixed(2)}`} sub="trailing 7 days" />
          <StatCard icon={AlertTriangle} label="Anomalies" value={`${anomalies.length}`} sub={anomalies.length > 0 ? 'agents over 2x average' : 'all normal'} />
        </div>

        {/* Anomalies */}
        {anomalies.length > 0 && (
          <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-4 space-y-2">
            <div className="flex items-center gap-2 text-amber-400 text-xs font-medium">
              <AlertTriangle className="h-3.5 w-3.5" />
              Spending Anomalies
            </div>
            {anomalies.map(a => (
              <div key={a.agent_id} className="flex items-center justify-between text-xs">
                <span className="text-foreground font-medium">{a.agent_key}</span>
                <span className="text-amber-400">
                  ${a.today_cost_usd.toFixed(2)} today ({a.ratio.toFixed(1)}x avg)
                </span>
              </div>
            ))}
          </div>
        )}

        {/* Per-agent breakdown */}
        <div className="space-y-2">
          <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Cost by Agent</h3>
          <div className="rounded-lg border border-border bg-card/50 p-4 space-y-1">
            {agents.length === 0 ? (
              <p className="text-xs text-muted-foreground">No spend data available</p>
            ) : (
              agents.slice(0, 15).map(a => <CostBar key={a.agent_id} agent={a} maxCost={maxAgentCost} />)
            )}
          </div>
        </div>

        {/* Daily trend */}
        {daily.length > 0 && (
          <div className="space-y-2">
            <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Daily Trend</h3>
            <div className="rounded-lg border border-border bg-card/50 p-4">
              <div className="flex items-end gap-1 h-24">
                {daily.map((d, i) => {
                  const maxDaily = Math.max(...daily.map(x => x.cost_usd), 0.01);
                  const h = (d.cost_usd / maxDaily) * 100;
                  return (
                    <div key={i} className="flex-1 flex flex-col items-center gap-0.5" title={`${d.date}: $${d.cost_usd.toFixed(2)}`}>
                      <div className="w-full rounded-t bg-emerald-500/60 transition-all" style={{ height: `${h}%`, minHeight: '2px' }} />
                    </div>
                  );
                })}
              </div>
              <div className="flex justify-between mt-1 text-2xs text-muted-foreground">
                <span>{daily[0]?.date}</span>
                <span>{daily[daily.length - 1]?.date}</span>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
