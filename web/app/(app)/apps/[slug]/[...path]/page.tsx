// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import AppDynamicClient from './client';

export const dynamic = 'force-static';

// App pages are registered at runtime — we provide a single placeholder path
// so the static exporter is satisfied. Actual routing is handled client-side.
export function generateStaticParams() {
  return [{ slug: '__app__', path: ['__page__'] }];
}

export default function AppDynamicPage() {
  return <AppDynamicClient />;
}
