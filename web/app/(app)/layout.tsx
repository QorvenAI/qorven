// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { QorvenLayout } from '@/components/layouts/qorven/layout';
import { AuthGuard } from '@/components/auth-guard';
import { ErrorBoundary } from '@/components/error-boundary';
import { AppHost } from '@/components/apps/app-host';
import type { ReactNode } from 'react';

export default function AppLayout({ children }: { children: ReactNode }) {
  return (
    <AuthGuard>
      <div className="qorven sidebar-fixed header-fixed w-full min-h-screen">
        <AppHost>
          <QorvenLayout>
            <ErrorBoundary>{children}</ErrorBoundary>
          </QorvenLayout>
        </AppHost>
      </div>
    </AuthGuard>
  );
}
