'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { IntegrationsTab } from '../integrations-tab';
import { PageShell } from '@/components/layouts/page-shell';

export default function IntegrationsPage() {
  return (
    <PageShell title="Data Integrations" description="LLM Stats and Artificial Analysis — model rankings, benchmarks, and pricing intelligence">
      <IntegrationsTab />
    </PageShell>
  );
}
