'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Legacy URL — sidebar canonical is /models-hub/router.
import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function RoutingPage() {
  const router = useRouter();
  useEffect(() => { router.replace('/models-hub/router'); }, [router]);
  return null;
}
