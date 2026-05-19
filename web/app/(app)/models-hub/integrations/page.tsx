'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { IntegrationsTab } from '../integrations-tab';
import { CanvasHeader } from '@/components/layouts/canvas-header';

export default function IntegrationsPage() {
  return (
    <div className="space-y-5">
      <CanvasHeader title="Data Integrations" description="LLM Stats and Artificial Analysis — model rankings, benchmarks, and pricing intelligence" />
      <IntegrationsTab />
    </div>
  );
}
