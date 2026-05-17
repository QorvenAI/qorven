'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { ErrorBoundary } from '@/components/error-boundary';

export default function ModelsHubLayout({ children }: { children: React.ReactNode }) {
  return <ErrorBoundary>{children}</ErrorBoundary>;
}
