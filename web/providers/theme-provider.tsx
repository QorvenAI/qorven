'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { type ReactNode, useEffect } from 'react';
import { ThemeProvider as CustomThemeProvider } from '@/lib/theme-provider';

export function ThemeProvider({ children }: { children: ReactNode }) {
  return <CustomThemeProvider>{children}</CustomThemeProvider>;
}
