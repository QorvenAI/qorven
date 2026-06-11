'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useRef, useCallback } from 'react';
import dynamic from 'next/dynamic';
import { Loader2 } from 'lucide-react';
import { detectLang } from './code-utils';
import { request } from '@/lib/api-core';
import { setMonaco } from '@/lib/monaco-models';
import type { LspDisposer } from '@/lib/lsp-client';

// LSP-capable language ids (must match lsp-client.ts LSP_LANGS)
const LSP_LANG_IDS = new Set(['go', 'typescript', 'javascript', 'python']);

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

export function CodeEditor({ content, path, onChange, projectId }: {
  content: string; path: string; onChange?: (v: string) => void; projectId?: string;
}) {
  const [isDark, setIsDark] = useState(true);
  const completionDisposable = useRef<{ dispose: () => void } | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  // LSP disposer — cleaned up on unmount or when (projectId, lang) changes
  const lspDisposerRef = useRef<LspDisposer | null>(null);

  useEffect(() => {
    const update = () => setIsDark(document.documentElement.classList.contains('dark'));
    update();
    const obs = new MutationObserver(update);
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
    return () => obs.disconnect();
  }, []);

  const handleEditorMount = useCallback((editor: any, monaco: any) => {
    // Seed the registry so syncModelContent / modelIsDirtyVsDisk work
    setMonaco(monaco);

    // ── LSP client (hover / goto-definition / popup-completion / diagnostics) ──
    // connectLSP is lazy-loaded and resolves gracefully if no server is available.
    // AI inline ghost-text (registerInlineCompletionsProvider below) coexists —
    // they use different Monaco APIs: popup completions vs inline suggestions.
    if (projectId) {
      const lang = detectLang(path);
      if (LSP_LANG_IDS.has(lang)) {
        // Dynamic import keeps lsp-client.ts out of the main bundle for non-IDE pages.
        import('@/lib/lsp-client').then(({ connectLSP }) => {
          connectLSP(projectId, lang, monaco)
            .then(disposer => { lspDisposerRef.current = disposer; })
            .catch(() => {}); // never let an LSP error break the editor
        }).catch(() => {});
      }
    }

    // Register inline completion provider for AI ghost text
    completionDisposable.current?.dispose();
    completionDisposable.current = monaco.languages.registerInlineCompletionsProvider('*', {
      provideInlineCompletions: async (model: any, position: any, _context: any, token: any) => {
        abortRef.current?.abort();
        const abort = new AbortController();
        abortRef.current = abort;

        token.onCancellationRequested(() => abort.abort());

        // 300ms debounce — don't fire on every keystroke
        await new Promise<void>((resolve, reject) => {
          const t = setTimeout(resolve, 300);
          abort.signal.addEventListener('abort', () => { clearTimeout(t); reject(); });
        }).catch(() => null);
        if (abort.signal.aborted) return { items: [] };

        const textUntilPosition = model.getValueInRange({
          startLineNumber: Math.max(1, position.lineNumber - 50),
          startColumn: 1,
          endLineNumber: position.lineNumber,
          endColumn: position.column,
        });

        const textAfterPosition = model.getValueInRange({
          startLineNumber: position.lineNumber,
          startColumn: position.column,
          endLineNumber: Math.min(model.getLineCount(), position.lineNumber + 10),
          endColumn: model.getLineMaxColumn(Math.min(model.getLineCount(), position.lineNumber + 10)),
        });

        if (textUntilPosition.trim().length < 3) return { items: [] };

        try {
          const res = await request('/code/completions', {
            method: 'POST',
            body: JSON.stringify({
              file_path: path,
              prefix: textUntilPosition.slice(-2000),
              suffix: textAfterPosition.slice(0, 500),
              language: detectLang(path),
              project_id: projectId || '',
            }),
            signal: abort.signal,
          }) as { completion?: string };

          if (!res.completion || abort.signal.aborted) return { items: [] };

          return {
            items: [{
              insertText: res.completion,
              range: {
                startLineNumber: position.lineNumber,
                startColumn: position.column,
                endLineNumber: position.lineNumber,
                endColumn: position.column,
              },
            }],
          };
        } catch {
          return { items: [] };
        }
      },
      freeInlineCompletions: () => {},
    });
  }, [path, projectId]);

  useEffect(() => {
    return () => {
      completionDisposable.current?.dispose();
      abortRef.current?.abort();
      lspDisposerRef.current?.();
      lspDisposerRef.current = null;
    };
  }, []);

  return (
    <MonacoEditor
      height="100%"
      path={path}
      defaultValue={content}
      language={detectLang(path)}
      theme={isDark ? 'vs-dark' : 'light'}
      keepCurrentModel={true}
      saveViewState={true}
      onChange={v => onChange?.(v ?? '')}
      onMount={handleEditorMount}
      options={{
        fontSize: 13, lineHeight: 20,
        fontFamily: '"JetBrains Mono", "Cascadia Code", "Fira Code", ui-monospace, monospace',
        fontLigatures: true, minimap: { enabled: true, scale: 1, showSlider: 'mouseover' },
        scrollBeyondLastLine: false, renderLineHighlight: 'line', cursorBlinking: 'smooth',
        cursorSmoothCaretAnimation: 'on', smoothScrolling: true, padding: { top: 6, bottom: 6 },
        folding: true, bracketPairColorization: { enabled: true },
        guides: { bracketPairs: 'active', indentation: true }, wordWrap: 'off',
        readOnly: !onChange, automaticLayout: true,
        scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8 },
        stickyScroll: { enabled: true },
        inlineSuggest: { enabled: true, mode: 'subwordSmart' },
        suggest: { preview: true, showInlineDetails: true },
      }}
    />
  );
}
