// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { ReactNode } from 'react';
import { Inter, JetBrains_Mono } from 'next/font/google';
import { cn } from '@/lib/utils';
import { ThemeProvider } from '@/providers/theme-provider';
import { WebSocketProvider } from '@/providers/websocket-provider';
import { Toaster } from '@/components/qor/toaster';
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

// Inline script that runs synchronously before React hydrates — reads the
// saved theme from localStorage and applies --primary to <html> immediately,
// preventing the flash where the page paints with the CSS-file default color
// before ThemeProvider's useEffect fires.
const themeScript = `(function(){try{var s=localStorage.getItem('qorven-theme');if(s){var t=JSON.parse(s);if(t&&t.primaryOklch){document.documentElement.style.setProperty('--primary',t.primaryOklch);document.documentElement.style.setProperty('--ring',t.primaryOklch);}}}catch(e){}})();`;

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html className="h-full dark" suppressHydrationWarning>
      {/* eslint-disable-next-line @next/next/no-before-interactive-script-outside-document */}
      <head>
        {/* Synchronous theme init — must run before first paint to avoid color flash */}
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body suppressHydrationWarning className={cn('antialiased flex h-full w-full text-sm text-foreground bg-background', inter.variable, jetbrains.variable, inter.className)}>
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
