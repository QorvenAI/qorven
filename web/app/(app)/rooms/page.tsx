'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Legacy redirect: the page moved to /hubs. Kept so old links/bookmarks resolve.
// Client-side because the production build is a static export (next.config
// redirects are dropped there).

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function RoomsRedirect() {
  const router = useRouter();
  useEffect(() => { router.replace('/hubs'); }, [router]);
  return null;
}
