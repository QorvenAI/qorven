'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import { providers as providersApi } from '@/lib/api';

export interface SelectedModel {
  provider_id: string;
  model_id: string;
  is_default: boolean;
  context_window?: number;
}

/** Fetches user-enabled generative models. Only these show in Soul model pickers. */
export function useSelectedModels() {
  const [models, setModels] = useState<SelectedModel[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    providersApi.availableModels('generative')
      .then((d) => { if (active) { setModels(Array.isArray(d) ? d : []); setLoading(false); } })
      .catch(() => {
        if (!active) return;
        providersApi.selectedModels()
          .then((d) => { if (active) { setModels(Array.isArray(d) ? d : []); setLoading(false); } })
          .catch(() => { if (active) setLoading(false); });
      });
    return () => { active = false; };
  }, []);

  const defaultModel = models.find((m) => m.is_default)?.model_id || models[0]?.model_id || '';

  return { models, defaultModel, loading };
}
