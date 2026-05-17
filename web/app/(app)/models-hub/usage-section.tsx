'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';

export default function UsageSection() {
  const [accountData, setAccountData] = useState<any>(null);
  const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

  useEffect(() => {
    fetch('/api/v1/usage/account', { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json()).then(setAccountData).catch(() => {});
  }, [getToken()]);

  const totalCost = accountData?.total_cost_this_month || 0;
  const souls = accountData?.souls || [];

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium mb-3">Account Usage This Month</h3>
        <div className="rounded-xl border border-border p-5 text-center">
          <p className="text-2xl font-semibold">${totalCost.toFixed(4)}</p>
          <p className="text-xs text-muted-foreground mt-1">Total spend this month</p>
        </div>
      </div>

      {souls.length > 0 && (
        <div>
          <h3 className="text-sm font-medium mb-3">Per-Agent Breakdown</h3>
          <div className="space-y-2">
            {souls.map((s: any) => {
              const pct = totalCost > 0 ? (s.cost / totalCost) * 100 : 0;
              return (
                <div key={s.id} className="flex items-center gap-3">
                  <div className={cn('flex h-7 w-7 items-center justify-center rounded-full bg-gradient-to-br text-2xs font-semibold text-white shrink-0', soulGradient(s.name))}>
                    {s.name?.charAt(0)}
                  </div>
                  <span className="text-xs w-28 truncate">{s.name}</span>
                  <div className="flex-1 h-2 rounded-full bg-muted overflow-hidden">
                    <div className="h-full rounded-full bg-primary" style={{ width: `${pct}%` }} />
                  </div>
                  <span className="text-xs text-muted-foreground w-20 text-right">${s.cost?.toFixed(4)} ({s.calls} calls)</span>
                </div>
              );
            })}
          </div>
        </div>
      )}

      <div>
        <h3 className="text-sm font-medium mb-2">Pricing Data</h3>
        <p className="text-2xs text-muted-foreground">2,023 models with pricing cached from LiteLLM. Auto-refreshes daily.</p>
        <button onClick={() => fetch('/api/v1/pricing/refresh', { method: 'POST', headers: { Authorization: `Bearer ${getToken()}` } }).then(() => alert('Pricing refreshed!'))}
          className="mt-2 rounded-lg border border-border px-3 py-1.5 text-xs hover:bg-accent">Refresh Pricing Now</button>
      </div>
    </div>
  );
}
