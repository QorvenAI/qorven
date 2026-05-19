'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { Zap, BarChart3 } from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { cn } from '@/lib/utils';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { request } from '@/lib/api-core';

type AgentCost = { agent_id: string; agent_name: string; total_cost_cents: number; total_input_tokens: number; total_output_tokens: number; call_count: number; budget_cents: number; used_cents: number };
type CostEvent = { id: number; agent_id: string; provider: string; model: string; input_tokens: number; output_tokens: number; cost_cents: number; created_at: string };

export default function BillingPage() {
  const [data, setData] = useState<{ agent_costs: AgentCost[]; total_cents: number; total_calls: number; recent: CostEvent[] } | null>(null);

  useEffect(() => { request<any>('/billing/costs').then(setData).catch(() => {}); }, []);

  if (!data) return <div className="p-6 text-muted-foreground">Loading...</div>;

  const totalDollars = (data.total_cents / 100).toFixed(2);

  // Empty state for fresh install
  // if (data.length === 0) return <EmptyState {...emptyStates.billing} />;
  return (
    <div className="p-6 max-w-6xl mx-auto space-y-6">
      <CanvasHeader title="Billing & Costs" description="Per-agent cost tracking and budget management" />

      <div className="grid grid-cols-3 gap-4">
        <div className="qr-card p-4">
          <p className="text-xs text-muted-foreground">Total Cost (30d)</p>
          <p className="text-2xl font-semibold">${totalDollars}</p>
        </div>
        <div className="qr-card p-4">
          <p className="text-xs text-muted-foreground">Total Calls</p>
          <p className="text-2xl font-semibold">{data.total_calls.toLocaleString()}</p>
        </div>
        <div className="qr-card p-4">
          <p className="text-xs text-muted-foreground">Active Agents</p>
          <p className="text-2xl font-semibold">{data.agent_costs?.length || 0}</p>
        </div>
      </div>

      <div>
        <h2 className="text-sm font-medium mb-2 flex items-center gap-2"><BarChart3 className="h-4 w-4" />Cost by Agent</h2>
        {/* Visual bar chart */}
        <div className="grid grid-cols-1 gap-1 mb-4">
          {(data.agent_costs || []).filter(a => a.call_count > 0).map(a => {
            const maxCost = Math.max(...(data.agent_costs || []).map(x => x.total_cost_cents), 1);
            const pct = (a.total_cost_cents / maxCost) * 100;
            return (
              <div key={a.agent_id} className="flex items-center gap-3 text-xs">
                <span className="w-24 text-right text-muted-foreground truncate">{a.agent_name}</span>
                <div className="flex-1 h-5 bg-muted/30 rounded overflow-hidden">
                  <div className="h-full bg-primary/70 rounded transition-all" style={{ width: `${Math.max(pct, 2)}%` }} />
                </div>
                <span className="w-16 text-right font-mono">${(a.total_cost_cents / 100).toFixed(3)}</span>
              </div>
            );
          })}
          {(data.agent_costs || []).filter(a => a.call_count > 0).length === 0 && (
            <p className="text-xs text-muted-foreground py-2">No usage data yet</p>
          )}
        </div>
        {/* Model distribution */}
        <div className="flex gap-2 flex-wrap mb-4">
          {Object.entries((data.recent || []).reduce((acc: Record<string, number>, e) => {
            acc[e.model] = (acc[e.model] || 0) + 1; return acc;
          }, {})).sort((a, b) => b[1] - a[1]).map(([model, count]) => (
            <span key={model} className="inline-flex items-center gap-1 rounded-full bg-muted/50 px-2 py-0.5 text-2xs">
              <span className="h-1.5 w-1.5 rounded-full bg-primary" />
              {model}: {count as number}
            </span>
          ))}
        </div>
        <div className="rounded-lg border border-border overflow-hidden">
          <table className="w-full text-sm">
            <thead><tr className="border-b border-border bg-muted/30">
              <th className="text-left px-3 py-2 text-xs text-muted-foreground">Agent</th>
              <th className="text-right px-3 py-2 text-xs text-muted-foreground">Calls</th>
              <th className="text-right px-3 py-2 text-xs text-muted-foreground">Input Tokens</th>
              <th className="text-right px-3 py-2 text-xs text-muted-foreground">Output Tokens</th>
              <th className="text-right px-3 py-2 text-xs text-muted-foreground">Cost</th>
              <th className="text-right px-3 py-2 text-xs text-muted-foreground">Budget</th>
            </tr></thead>
            <tbody>
              {(data.agent_costs || []).map(a => {
                const pct = a.budget_cents > 0 ? Math.min(100, (a.used_cents / a.budget_cents) * 100) : 0;
                return (
                  <tr key={a.agent_id} className="border-b border-border/50">
                    <td className="px-3 py-2 font-medium">{a.agent_name || a.agent_id.slice(0,8)}</td>
                    <td className="px-3 py-2 text-right text-muted-foreground">{a.call_count}</td>
                    <td className="px-3 py-2 text-right text-muted-foreground">{a.total_input_tokens.toLocaleString()}</td>
                    <td className="px-3 py-2 text-right text-muted-foreground">{a.total_output_tokens.toLocaleString()}</td>
                    <td className="px-3 py-2 text-right font-mono">${(a.total_cost_cents / 100).toFixed(4)}</td>
                    <td className="px-3 py-2 text-right">
                      {a.budget_cents > 0 ? (
                        <div className="flex items-center gap-2 justify-end">
                          <div className="w-16 h-1.5 bg-muted rounded-full overflow-hidden">
                            <div className={cn('h-full rounded-full', pct > 90 ? 'bg-red-500' : pct > 70 ? 'bg-yellow-500' : 'bg-emerald-500')} style={{ width: `${pct}%` }} />
                          </div>
                          <span className="text-2xs text-muted-foreground">{pct.toFixed(0)}%</span>
                        </div>
                      ) : <span className="text-2xs text-muted-foreground">No limit</span>}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      <div>
        <h2 className="text-sm font-medium mb-2 flex items-center gap-2"><Zap className="h-4 w-4" />Recent Calls</h2>
        <div className="space-y-1">
          {(data.recent || []).slice(0, 20).map(e => (
            <div key={e.id} className="flex items-center justify-between rounded border border-border/50 px-3 py-1.5 text-xs">
              <div className="flex items-center gap-3">
                <span className="text-muted-foreground">{new Date(e.created_at).toLocaleTimeString()}</span>
                <span className="font-mono text-muted-foreground">{e.model}</span>
              </div>
              <div className="flex items-center gap-4">
                <span className="text-muted-foreground">{e.input_tokens}→{e.output_tokens} tok</span>
                <span className="font-mono">${(e.cost_cents / 100).toFixed(4)}</span>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
