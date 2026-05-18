'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { isAuthenticated } from '@/lib/api';
import { Loader2 } from 'lucide-react';

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [checked, setChecked] = useState(false);

  useEffect(() => {
    if (!isAuthenticated()) {
      router.replace('/login');
    } else {
      // Backfill cookie from localStorage so middleware can read it
      // (covers existing sessions that pre-date the cookie approach)
      const token = localStorage.getItem('qorven_token');
      if (token && !document.cookie.includes('qorven_token=')) {
        document.cookie = `qorven_token=${token}; path=/; max-age=${7 * 24 * 3600}; SameSite=Lax`;
      }
      setChecked(true);
    }
  }, [router]);

  if (!checked) {
    return (
      <div className="flex h-screen w-full items-center justify-center bg-background">
        <Loader2 className="h-6 w-6 animate-spin text-primary" />
      </div>
    );
  }

  return <>{children}</>;
}
