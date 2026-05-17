// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import Client from './client';

export const dynamic = 'force-static';

export function generateStaticParams() {
  return [{ id: '__dynamic__' }];
}

export default function Page() {
  return <Client />;
}
