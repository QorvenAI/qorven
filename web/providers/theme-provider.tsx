'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { type ReactNode, useEffect } from 'react';
import { ThemeProvider as CustomThemeProvider } from '@/lib/theme-provider';

export function ThemeProvider({ children }: { children: ReactNode }) {
  return <CustomThemeProvider>{children}</CustomThemeProvider>;
}
