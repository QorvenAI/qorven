'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useCallback } from 'react';
import { Loader2 } from 'lucide-react';
import { providers as providersApi } from '@/lib/api';
import { RoutingTab } from '../routing-tab';

type SelModel = { model_id: string; provider_id: string; is_default?: boolean };

export default function RouterPage() {
  const [selectedModels, setSelectedModels] = useState<SelModel[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(() => {
    providersApi.selectedModels()
      .then(d => setSelectedModels(Array.isArray(d) ? d : []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="space-y-5">
      <div className="pb-2">
        <h1 className="text-lg font-semibold leading-none">Model Router</h1>
        <p className="text-sm text-muted-foreground mt-1">Assign models to work categories and review SmartRouter decisions</p>
      </div>
      {loading ? (
        <div className="flex items-center gap-2 py-12 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading…
        </div>
      ) : (
        <RoutingTab selectedModels={selectedModels} />
      )}
    </div>
  );
}
