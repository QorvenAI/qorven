'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function ApprovalsPage() {
  const router = useRouter();
  useEffect(() => {
    router.replace('/code?tab=inbox');
  }, [router]);
  return null;
}
