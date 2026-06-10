'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Legacy redirect: /rooms/[id] → /hubs/[id]. Preserves the hub id.

import { useEffect } from 'react';
import { useRouter, useParams } from 'next/navigation';

export default function Redirect() {
  const router = useRouter();
  const params = useParams();
  useEffect(() => {
    const id = params?.id;
    router.replace(id && id !== '__dynamic__' ? `/hubs/${id}` : '/hubs');
  }, [router, params]);
  return null;
}
