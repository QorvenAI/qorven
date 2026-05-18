'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { IntegrationsTab } from '../integrations-tab';

export default function IntegrationsPage() {
  return (
    <div className="space-y-5">
      <div className="pb-2">
        <h1 className="text-lg font-semibold leading-none">Data Integrations</h1>
        <p className="text-sm text-muted-foreground mt-1">
          LLM Stats and Artificial Analysis — model rankings, benchmarks, and pricing intelligence
        </p>
      </div>
      <IntegrationsTab />
    </div>
  );
}
