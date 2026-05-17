'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import { providers as providersApi } from '@/lib/api';
import { useSelectedModels } from '@/hooks/use-selected-models';
import { cn } from '@/lib/utils';
import { Star } from 'lucide-react';

const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

export default function ModelsSection() {
  const { models } = useSelectedModels();
  const [catalog, setCatalog] = useState<any[]>([]);

  useEffect(() => {
    fetch('/api/v1/models/catalog', { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json()).then((d) => setCatalog(Array.isArray(d) ? d : [])).catch(() => {});
  }, []);

  const selectedIds = new Set(models.map((m) => m.model_id));
  const defaultModel = models.find((m) => m.is_default)?.model_id;

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium mb-1">Selected Models ({models.length})</h3>
        <p className="text-2xs text-muted-foreground mb-3">These models are available for your agents. Manage per provider in the Providers tab.</p>
        {models.length === 0 ? (
          <p className="text-sm text-muted-foreground py-4">No models selected — go to Providers tab, add a key, then select models</p>
        ) : (
          <div className="space-y-1">
            {models.map((m) => (
              <div key={m.model_id} className="flex items-center gap-3 rounded-lg border border-border px-4 py-2.5">
                <span className="text-sm font-medium flex-1">{m.model_id}</span>
                {m.is_default && <span className="flex items-center gap-1 text-amber-400 text-xs"><Star className="h-3 w-3 fill-amber-400" /> Default</span>}
                <span className="text-2xs text-muted-foreground">{m.provider_id}</span>
              </div>
            ))}
          </div>
        )}
      </div>

      {catalog.length > 0 && (
        <div>
          <h3 className="text-sm font-medium mb-1">Recommended Models</h3>
          <p className="text-2xs text-muted-foreground mb-3">Based on benchmarks and pricing</p>
          <div className="grid grid-cols-2 gap-2">
            {catalog.filter((m: any) => m.is_recommended).map((m: any) => (
              <div key={m.id} className={cn('rounded-xl border p-3', selectedIds.has(m.id) ? 'border-primary/40 bg-primary/5' : 'border-border')}>
                <p className="text-xs font-medium">{m.name}</p>
                <p className="text-2xs text-muted-foreground">{m.description}</p>
                <div className="flex gap-2 mt-1 text-2xs text-muted-foreground">
                  {m.context_window && <span>{(m.context_window/1000).toFixed(0)}K</span>}
                  {m.input_cost_per_1m > 0 && <span>${m.input_cost_per_1m}/${m.output_cost_per_1m}</span>}
                  <span className={cn('rounded-full px-1.5', m.tier === 'free' ? 'bg-emerald-400/10 text-emerald-400' : m.tier === 'cheap' ? 'bg-blue-400/10 text-blue-400' : 'bg-primary/10 text-primary/70')}>{m.tier}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
