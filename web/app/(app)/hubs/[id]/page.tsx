// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import Client from './client';

export const dynamic = 'force-static';
export function generateStaticParams() {
  return [{ id: '__dynamic__' }];
}

export default function Page() {
  return <Client />;
}
