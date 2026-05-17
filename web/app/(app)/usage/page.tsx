'use client'

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react'
import { BarChart3, Loader2, AlertCircle, DollarSign, Cpu } from 'lucide-react'
import { EmptyState, emptyStates } from '@/components/empty-state';
import { usage as usageApi } from '@/lib/api';

interface CostData {
  total_cost_this_month: number
  souls: { id: string; name: string; cost: number; calls: number }[]
}

export default function UsagePage() {
  const [data, setData] = useState<CostData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    usageApi.account()
      .then(d => setData(d))
      .catch(e => setError(e instanceof Error ? e.message : 'Failed to load'))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return (
    <div className="flex items-center justify-center py-20">
      <Loader2 className="w-6 h-6 text-primary animate-spin" />
    </div>
  )

  if (!loading && !error && !data) return (
    <div className="p-6"><EmptyState {...emptyStates.usage} /></div>
  );

  if (error) return (
    <div className="flex items-center justify-center py-20 text-destructive gap-2">
      <AlertCircle className="w-5 h-5" />{error}
    </div>
  )

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3 mb-8">
        <BarChart3 className="w-7 h-7 text-primary" />
        <h1 className="text-lg font-semibold">Usage &amp; Costs</h1>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-8">
        <div className="border border-border rounded-lg p-5 flex items-center gap-4">
          <DollarSign className="w-8 h-8 text-primary" />
          <div>
            <p className="text-sm text-muted-foreground">Cost This Month</p>
            <p className="text-xl font-bold">${data?.total_cost_this_month?.toFixed(4)}</p>
          </div>
        </div>
        <div className="border border-border rounded-lg p-5 flex items-center gap-4">
          <Cpu className="w-8 h-8 text-primary" />
          <div>
            <p className="text-sm text-muted-foreground">Active Agents</p>
            <p className="text-lg font-semibold">{data?.souls?.length ?? 0}</p>
          </div>
        </div>
      </div>
      <div className="border border-border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-card">
            <tr>
              <th className="text-left p-3 text-muted-foreground font-medium">Agent</th>
              <th className="text-right p-3 text-muted-foreground font-medium">Calls</th>
              <th className="text-right p-3 text-muted-foreground font-medium">Cost</th>
            </tr>
          </thead>
          <tbody>
            {data?.souls?.map((a) => (
              <tr key={a.id} className="border-t border-border hover:bg-card/50">
                <td className="p-3">{a.name || a.id.slice(0, 8)}</td>
                <td className="p-3 text-right text-muted-foreground">{a.calls}</td>
                <td className="p-3 text-right text-primary">${a.cost.toFixed(4)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {!data?.souls?.length && <p className="text-muted-foreground text-center py-8">No usage data.</p>}
      </div>
    </div>
  )
}
