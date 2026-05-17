'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, type ReactNode } from 'react';
import { useToolbar } from '@/components/layouts/qorven/toolbar';

export function useToolbarContent(left?: ReactNode, right?: ReactNode) {
  const { setLeft, setRight } = useToolbar();
  useEffect(() => {
    if (left !== undefined) setLeft(left);
    if (right !== undefined) setRight(right);
    return () => { setLeft(null); setRight(null); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [left, right]);
}
