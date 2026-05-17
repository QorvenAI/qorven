'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import dynamic from 'next/dynamic';
import { Loader2 } from 'lucide-react';
import { detectLang } from './code-utils';

const MonacoEditor = dynamic(
  () => import('@monaco-editor/react').then(m => m.default),
  {
    ssr: false,
    loading: () => (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
      </div>
    ),
  }
);

export function CodeEditor({ content, path, onChange }: {
  content: string; path: string; onChange?: (v: string) => void;
}) {
  const [isDark, setIsDark] = useState(true);
  useEffect(() => {
    const update = () => setIsDark(document.documentElement.classList.contains('dark'));
    update();
    const obs = new MutationObserver(update);
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
    return () => obs.disconnect();
  }, []);
  return (
    <MonacoEditor
      height="100%"
      language={detectLang(path)}
      value={content}
      theme={isDark ? 'vs-dark' : 'light'}
      onChange={v => onChange?.(v ?? '')}
      options={{
        fontSize: 13, lineHeight: 20,
        fontFamily: '"JetBrains Mono", "Cascadia Code", "Fira Code", ui-monospace, monospace', // ok — Monaco editor font
        fontLigatures: true, minimap: { enabled: true, scale: 1, showSlider: 'mouseover' },
        scrollBeyondLastLine: false, renderLineHighlight: 'line', cursorBlinking: 'smooth',
        cursorSmoothCaretAnimation: 'on', smoothScrolling: true, padding: { top: 6, bottom: 6 },
        folding: true, bracketPairColorization: { enabled: true },
        guides: { bracketPairs: 'active', indentation: true }, wordWrap: 'off',
        readOnly: !onChange, automaticLayout: true,
        scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8 },
        stickyScroll: { enabled: true },
      }}
    />
  );
}
