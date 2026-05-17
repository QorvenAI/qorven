'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function ModelsHubPage() {
  const router = useRouter();
  useEffect(() => { router.replace('/models-hub/generative'); }, [router]);
  return null;
}
