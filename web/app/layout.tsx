// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { ReactNode } from 'react';
import { Inter, JetBrains_Mono } from 'next/font/google';
import { cn } from '@/lib/utils';
import { ThemeProvider } from '@/providers/theme-provider';
import { WebSocketProvider } from '@/providers/websocket-provider';
import { Toaster } from 'sonner';
import { CommandPalette } from '@/components/modals/command-palette';
import type { Metadata } from 'next';

import '@/css/styles.css';
import 'katex/dist/katex.min.css';

const inter = Inter({ subsets: ['latin'], weight: ['400', '500', '600', '700'], variable: '--font-inter' });
const jetbrains = JetBrains_Mono({ subsets: ['latin'], variable: '--font-mono' });

export const metadata: Metadata = {
  title: { template: '%s | Qorven', default: 'Qorven' },
  description: 'AI Agent Platform',
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html className="h-full dark" suppressHydrationWarning>
      <body className={cn('antialiased flex h-full w-full text-sm text-foreground bg-background', inter.variable, jetbrains.variable, inter.className)}>
        <ThemeProvider>
          <WebSocketProvider>
            {children}
            <Toaster position="bottom-right" richColors />
            <CommandPalette />
          </WebSocketProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
