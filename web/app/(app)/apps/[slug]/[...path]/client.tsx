'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import React from 'react';
import { useParams } from 'next/navigation';
import { Package, AlertTriangle } from 'lucide-react';
import { useAppRegistry } from '@/components/apps/app-registry-context';
import { ErrorBoundary } from '@/components/error-boundary';
import { request, getToken } from '@/lib/api-core';

export default function AppDynamicClient() {
  const params = useParams<{ slug: string; path: string[] }>();
  const { entries } = useAppRegistry();

  const slug = params.slug;
  const pathStr = Array.isArray(params.path) ? params.path.join('/') : params.path ?? '';

  const app = entries[slug];
  if (!app) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-center">
        <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-muted mb-4">
          <Package className="h-7 w-7 text-muted-foreground" />
        </div>
        <p className="font-medium">App not loaded</p>
        <p className="text-sm text-muted-foreground mt-1">
          &ldquo;{slug}&rdquo; is not registered. It may be disabled or still loading.
        </p>
      </div>
    );
  }

  const page = app.pages?.find((p) => p.path === pathStr);
  if (!page) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-center">
        <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-muted mb-4">
          <AlertTriangle className="h-7 w-7 text-muted-foreground" />
        </div>
        <p className="font-medium">Page not found</p>
        <p className="text-sm text-muted-foreground mt-1">
          No page at path &ldquo;{pathStr}&rdquo; in app &ldquo;{slug}&rdquo;.
        </p>
      </div>
    );
  }

  return (
    <ErrorBoundary>
      {React.createElement(page.component, {
        React,
        request: (path: string, init?: RequestInit) => request(path, init),
        token: getToken(),
        appId: slug,
      })}
    </ErrorBoundary>
  );
}
