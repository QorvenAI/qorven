'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { Cpu, Wrench, RefreshCw } from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { cn } from '@/lib/utils';
import { request } from '@/lib/api-core';

export default function SystemPage() {
  const [supervisor, setSupervisor] = useState<any>(null);
  const [health, setHealth] = useState<any[]>([]);
  const [metrics, setMetrics] = useState<any>(null);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    request<any>('/supervisor/status').then(setSupervisor).catch(() => {});
    request<any>('/supervisor/health').then(d => setHealth(d?.agents || [])).catch(() => {});
    request<any>('/tools/metrics').then(setMetrics).catch(() => {});
    const interval = setInterval(() => setTick(t => t + 1), 30000);
    return () => clearInterval(interval);
  }, [tick]);

  return (
    <div className="p-6 max-w-6xl mx-auto space-y-6">
      <CanvasHeader title="System Knowledge" description="Auto-generated from codebase · refreshes every 30s"
        actions={<button onClick={() => setTick(t => t + 1)} className="text-muted-foreground hover:text-foreground/90 cursor-pointer"><RefreshCw className="h-4 w-4" /></button>}
      />

      {/* Supervisor stats */}
      {supervisor && (
        <div className="grid grid-cols-4 gap-4">
          {[
            { label: 'Exchanges', value: supervisor.total_exchanges },
            { label: 'Open', value: supervisor.open_exchanges },
            { label: 'ACKed', value: supervisor.acked_exchanges },
            { label: 'Escalations', value: supervisor.pending_escalations },
          ].map(s => (
            <div key={s.label} className="rounded-xl border border-border bg-card/50 p-4">
              <p className="text-xs text-muted-foreground">{s.label}</p>
              <p className="text-2xl font-semibold mt-1">{s.value ?? 0}</p>
            </div>
          ))}
        </div>
      )}

      {/* Agent health */}
      {health.length > 0 && (
        <div>
          <h2 className="text-sm font-semibold mb-2 flex items-center gap-2"><Cpu className="h-4 w-4" />Agent Health</h2>
          <div className="rounded-xl border border-border overflow-hidden">
            <table className="w-full text-sm">
              <thead><tr className="border-b border-border bg-card/50">
                <th className="text-left px-4 py-2 font-medium text-muted-foreground">Agent</th>
                <th className="text-left px-4 py-2 font-medium text-muted-foreground">Status</th>
                <th className="text-right px-4 py-2 font-medium text-muted-foreground">Errors</th>
                <th className="text-right px-4 py-2 font-medium text-muted-foreground">Sampling</th>
              </tr></thead>
              <tbody>
                {health.map((a: any) => (
                  <tr key={a.agent_id} className="border-b border-border/50">
                    <td className="px-4 py-2">{a.agent_name || a.agent_id?.slice(0, 8)}</td>
                    <td className="px-4 py-2">
                      <span className={cn('px-2 py-0.5 rounded-full text-xs',
                        a.status === 'healthy' ? 'bg-emerald-500/10 text-emerald-400' :
                        a.status === 'degraded' ? 'bg-amber-500/10 text-amber-400' :
                        'bg-red-500/10 text-red-400')}>{a.status}</span>
                    </td>
                    <td className="px-4 py-2 text-right">{a.consecutive_errors}</td>
                    <td className="px-4 py-2 text-right">{((a.sampling_rate || 0) * 100).toFixed(0)}%</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Tool metrics */}
      {metrics?.tools?.length > 0 && (
        <div>
          <h2 className="text-sm font-semibold mb-2 flex items-center gap-2"><Wrench className="h-4 w-4" />Tool Metrics</h2>
          <div className="rounded-xl border border-border overflow-hidden">
            <table className="w-full text-sm">
              <thead><tr className="border-b border-border bg-card/50">
                <th className="text-left px-4 py-2 font-medium text-muted-foreground">Tool</th>
                <th className="text-right px-4 py-2 font-medium text-muted-foreground">Calls</th>
                <th className="text-right px-4 py-2 font-medium text-muted-foreground">Success</th>
                <th className="text-right px-4 py-2 font-medium text-muted-foreground">Avg Latency</th>
              </tr></thead>
              <tbody>
                {metrics.tools.map((t: any) => (
                  <tr key={t.name} className="border-b border-border/50">
                    <td className="px-4 py-2 font-mono text-xs">{t.name}</td>
                    <td className="px-4 py-2 text-right">{t.call_count}</td>
                    <td className="px-4 py-2 text-right">{((t.success_rate || 0) * 100).toFixed(0)}%</td>
                    <td className="px-4 py-2 text-right">{(t.avg_latency_ms || 0).toFixed(0)}ms</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
