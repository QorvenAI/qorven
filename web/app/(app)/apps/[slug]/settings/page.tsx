// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import AppSettingsClient from './client';

export const dynamic = 'force-static';

export function generateStaticParams() {
  return [{ slug: '__app__' }];
}

export default function AppSettingsPage() {
  return <AppSettingsClient />;
}
