// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Server shim for the fully-client-rendered Qor detail page.
// Static export (output: 'export') needs generateStaticParams for
// every dynamic route. The real id isn't known at build time, so we
// emit one placeholder shell; the Go gateway rewrites /qors/<real>
// to this same shell at request time. The actual React tree is
// fully client-rendered and reads the id from useParams, so no
// server-side content is tied to the placeholder.
import Client from './client';

export const dynamic = 'force-static';
export function generateStaticParams() {
  return [{ id: '__dynamic__' }];
}

export default function Page() {
  return <Client />;
}
