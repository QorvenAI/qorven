// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * LSP client bridge — connects Monaco to the backend LSP relay over WebSocket.
 *
 * Architecture note:
 *   `monaco-languageclient` (any version) requires `@codingame/monaco-vscode-api` which
 *   completely replaces the monaco instance — fundamentally incompatible with
 *   @monaco-editor/react's bundled monaco. We therefore implement a lightweight
 *   LSP-over-WebSocket client directly using:
 *     - `vscode-ws-jsonrpc` (already in package.json) for the WS↔JSON-RPC transport
 *     - `vscode-jsonrpc` createMessageConnection for request/notification plumbing
 *   then wire LSP responses (hover, definition, completion, publishDiagnostics) into
 *   Monaco's provider/marker APIs using the monaco instance from onMount.
 *
 * WS URL: wsBase('/v1/lsp/{projectId}?lang={lang}&token={token}')
 *   - wsBase() (api-url.ts:65) returns ws://localhost:4200 in dev (no /api prefix)
 *   - ?token= matches the terminal WS auth pattern (gw.wsAuth gate)
 *   - No /api prefix — LSP route is served directly by Go gateway under /v1/
 *
 * One-client-per-(projectId,lang) guard: module-level Map keyed by `${projectId}:${lang}`.
 *
 * Degradation: if the WS closes before/after `initialize` (no server for language),
 * the promise resolves to a no-op disposer — never throws, editor keeps AI ghost-text.
 */

import { wsBase } from './api-url';
import { getToken } from './api-core';

// ─── LSP method constants (inlined to avoid importing the full protocol package) ───
const M = {
  initialize:          'initialize',
  initialized:         'initialized',
  shutdown:            'shutdown',
  exit:                'exit',
  textOpen:            'textDocument/didOpen',
  textChange:          'textDocument/didChange',
  textClose:           'textDocument/didClose',
  hover:               'textDocument/hover',
  definition:          'textDocument/definition',
  completion:          'textDocument/completion',
  publishDiagnostics:  'textDocument/publishDiagnostics',
} as const;

/** Disposer — closes WS + unregisters Monaco providers. */
export type LspDisposer = () => void;

/** Map<projectId:lang, disposer> — guards one client per (project, language). */
const _clients = new Map<string, LspDisposer>();

/**
 * LSP-capable languages. Monaco lang id → LSP languageId (same for all supported langs).
 * Silently skips anything not in this set.
 */
const LSP_LANGS = new Set(['go', 'typescript', 'javascript', 'python']);

/**
 * connectLSP — lazy-loads transport deps, opens WS, wires hover/definition/completion
 * and publishDiagnostics into the supplied Monaco instance.
 *
 * @param projectId  The /code project UUID.
 * @param lang       Monaco language id (go | typescript | javascript | python).
 * @param monaco     The monaco instance from handleEditorMount — must be the same
 *                   instance @monaco-editor/react uses (do NOT import monaco separately).
 * @returns          A disposer that stops the client and closes the WS.
 */
export async function connectLSP(
  projectId: string,
  lang: string,
  monaco: any,
): Promise<LspDisposer> {
  if (!LSP_LANGS.has(lang)) return () => {};

  const key = `${projectId}:${lang}`;
  const existing = _clients.get(key);
  if (existing) return existing;

  // Lazy-import transport — these are large; don't pay cost on non-IDE pages.
  let createWebSocketConnection: any;
  let toSocket: any;
  try {
    const wsJsonrpc = await import('vscode-ws-jsonrpc');
    // vscode-ws-jsonrpc re-exports from ./socket/connection and ./connection
    createWebSocketConnection = wsJsonrpc.createWebSocketConnection;
    toSocket                  = wsJsonrpc.toSocket;
  } catch {
    // Package not available (should not happen in production — pnpm add succeeded).
    return () => {};
  }

  const token = getToken();
  const url   = wsBase(`/v1/lsp/${encodeURIComponent(projectId)}?lang=${lang}&token=${encodeURIComponent(token)}`);

  return new Promise<LspDisposer>((resolve) => {
    let ws: WebSocket;
    try {
      ws = new WebSocket(url);
    } catch {
      resolve(() => {});
      return;
    }

    // Accumulated Monaco provider disposables + WS state
    const disposables: Array<{ dispose(): void }> = [];
    let   conn: any        = null;
    let   initialized      = false;
    let   closed           = false;

    // ── Graceful no-op disposer if WS closes before we finish init ──
    function makeDisposer(): LspDisposer {
      return () => {
        closed = true;
        _clients.delete(key);
        disposables.forEach(d => { try { d.dispose(); } catch {} });
        try { conn?.sendNotification(M.shutdown); } catch {}
        try { ws.close(); } catch {}
        // Clear diagnostics for all models of this language
        try {
          monaco.editor.getModels()
            .filter((m: any) => m.getLanguageId() === lang)
            .forEach((m: any) => monaco.editor.setModelMarkers(m, 'lsp', []));
        } catch {}
      };
    }

    ws.onclose = () => {
      if (!initialized) {
        // Server not available for this language — resolve with no-op
        resolve(() => {});
      }
      // If already initialized, makeDisposer was already registered
    };

    ws.onerror = () => {
      if (!initialized) resolve(() => {});
    };

    ws.onopen = async () => {
      try {
        // Wrap the native WS in vscode-ws-jsonrpc's IWebSocket shim
        const socket = toSocket(ws);
        // createWebSocketConnection comes from vscode-ws-jsonrpc socket/connection
        conn = createWebSocketConnection(socket, { log() {}, warn() {}, error() {}, info() {} });
        conn.listen();

        // ── LSP initialize handshake ──
        await conn.sendRequest(M.initialize, {
          processId: null,
          clientInfo: { name: 'qorven-ide', version: '1.0' },
          rootUri: null,
          capabilities: {
            textDocument: {
              synchronization: { dynamicRegistration: false, willSave: false, didSave: false, willSaveWaitUntil: false },
              completion: {
                dynamicRegistration: false,
                completionItem: {
                  snippetSupport:          true,
                  commitCharactersSupport: false,
                  documentationFormat:     ['markdown', 'plaintext'],
                  deprecatedSupport:       true,
                  preselectSupport:        false,
                },
              },
              hover: { dynamicRegistration: false, contentFormat: ['markdown', 'plaintext'] },
              definition: { dynamicRegistration: false, linkSupport: false },
              publishDiagnostics: { relatedInformation: true },
            },
            workspace: { applyEdit: false },
          },
          workspaceFolders: null,
        });

        conn.sendNotification(M.initialized, {});
        initialized = true;

        // ── Open all currently-loaded models for this language ──
        const openModels = new Set<string>();
        for (const model of monaco.editor.getModels()) {
          if (model.getLanguageId() !== lang) continue;
          const uri   = model.uri.toString();
          const text  = model.getValue();
          openModels.add(uri);
          conn.sendNotification(M.textOpen, {
            textDocument: { uri, languageId: lang, version: 1, text },
          });
        }

        // ── Track new model opens / version changes ──
        const onModelAdd = monaco.editor.onDidCreateModel((model: any) => {
          if (closed || model.getLanguageId() !== lang) return;
          const uri = model.uri.toString();
          openModels.add(uri);
          conn.sendNotification(M.textOpen, {
            textDocument: { uri, languageId: lang, version: 1, text: model.getValue() },
          });
          // Track edits
          const editSub = model.onDidChangeContent((e: any) => {
            if (closed) return;
            conn.sendNotification(M.textChange, {
              textDocument: { uri, version: e.versionId },
              contentChanges: [{ text: model.getValue() }],
            });
          });
          disposables.push(editSub);
          // Track dispose
          const dispSub = model.onWillDispose(() => {
            openModels.delete(uri);
            if (!closed) conn.sendNotification(M.textClose, { textDocument: { uri } });
          });
          disposables.push(dispSub);
        });
        disposables.push(onModelAdd);

        // Track edits for models already open
        for (const model of monaco.editor.getModels()) {
          if (model.getLanguageId() !== lang) continue;
          const uri = model.uri.toString();
          const editSub = model.onDidChangeContent((e: any) => {
            if (closed) return;
            conn.sendNotification(M.textChange, {
              textDocument: { uri, version: e.versionId },
              contentChanges: [{ text: model.getValue() }],
            });
          });
          disposables.push(editSub);
          const dispSub = model.onWillDispose(() => {
            openModels.delete(uri);
            if (!closed) conn.sendNotification(M.textClose, { textDocument: { uri } });
          });
          disposables.push(dispSub);
        }

        // ── publishDiagnostics → Monaco markers ──
        conn.onNotification(M.publishDiagnostics, (params: any) => {
          if (closed) return;
          try {
            const model = monaco.editor.getModel(monaco.Uri.parse(params.uri));
            if (!model) return;
            const markers = (params.diagnostics ?? []).map((d: any) => ({
              severity:        lspSeverityToMonaco(monaco, d.severity ?? 1),
              startLineNumber: (d.range?.start?.line ?? 0) + 1,
              startColumn:     (d.range?.start?.character ?? 0) + 1,
              endLineNumber:   (d.range?.end?.line ?? 0) + 1,
              endColumn:       (d.range?.end?.character ?? 0) + 1,
              message:         d.message ?? '',
              source:          d.source ?? lang,
              code:            d.code != null ? String(d.code) : undefined,
            }));
            monaco.editor.setModelMarkers(model, 'lsp', markers);
          } catch {}
        });

        // ── Hover provider ──
        const hoverDisp = monaco.languages.registerHoverProvider(lang, {
          provideHover: async (model: any, position: any) => {
            if (closed) return null;
            try {
              const result = await conn.sendRequest(M.hover, {
                textDocument: { uri: model.uri.toString() },
                position:     { line: position.lineNumber - 1, character: position.column - 1 },
              });
              if (!result?.contents) return null;
              const contents = normalizeMarkup(result.contents);
              if (!contents.length) return null;
              return {
                contents,
                range: result.range ? lspRangeToMonaco(result.range) : undefined,
              };
            } catch {
              return null;
            }
          },
        });
        disposables.push(hoverDisp);

        // ── Goto-definition provider ──
        const defDisp = monaco.languages.registerDefinitionProvider(lang, {
          provideDefinition: async (model: any, position: any) => {
            if (closed) return null;
            try {
              const result = await conn.sendRequest(M.definition, {
                textDocument: { uri: model.uri.toString() },
                position:     { line: position.lineNumber - 1, character: position.column - 1 },
              });
              if (!result) return null;
              const locs = Array.isArray(result) ? result : [result];
              return locs.map((loc: any) => ({
                uri:   monaco.Uri.parse(loc.uri ?? loc.targetUri),
                range: lspRangeToMonaco(loc.range ?? loc.targetSelectionRange ?? loc.targetRange),
              }));
            } catch {
              return null;
            }
          },
        });
        disposables.push(defDisp);

        // ── Completion provider ──
        // Coexists with the AI inline-completions provider (registerInlineCompletionsProvider).
        // This registers a *popup* completion provider (Ctrl+Space / auto-trigger) —
        // a different API from the AI ghost-text inline provider. Both are active.
        const completionDisp = monaco.languages.registerCompletionItemProvider(lang, {
          triggerCharacters: ['.', ':', '"', "'", '/', '(', ',', ' '],
          provideCompletionItems: async (model: any, position: any) => {
            if (closed) return { suggestions: [] };
            try {
              const result = await conn.sendRequest(M.completion, {
                textDocument: { uri: model.uri.toString() },
                position:     { line: position.lineNumber - 1, character: position.column - 1 },
                context:      { triggerKind: 1 },
              });
              if (!result) return { suggestions: [] };
              const items = Array.isArray(result) ? result : (result.items ?? []);
              const word  = model.getWordUntilPosition(position);
              const range = {
                startLineNumber: position.lineNumber,
                endLineNumber:   position.lineNumber,
                startColumn:     word.startColumn,
                endColumn:       word.endColumn,
              };
              return {
                suggestions: items.map((item: any) => lspCompletionToMonaco(monaco, item, range)),
              };
            } catch {
              return { suggestions: [] };
            }
          },
        });
        disposables.push(completionDisp);

        // Register disposer in the global map
        const disposer = makeDisposer();
        _clients.set(key, disposer);
        resolve(disposer);
      } catch {
        // init failed (server closed WS, protocol error, etc.) — resolve gracefully
        resolve(() => { _clients.delete(key); });
      }
    };
  });
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function lspRangeToMonaco(r: any) {
  return {
    startLineNumber: (r.start?.line ?? 0) + 1,
    startColumn:     (r.start?.character ?? 0) + 1,
    endLineNumber:   (r.end?.line ?? 0) + 1,
    endColumn:       (r.end?.character ?? 0) + 1,
  };
}

function lspSeverityToMonaco(monaco: any, severity: number): number {
  // LSP: 1=Error 2=Warning 3=Information 4=Hint
  // Monaco MarkerSeverity: Error=8 Warning=4 Info=2 Hint=1
  switch (severity) {
    case 1:  return monaco.MarkerSeverity.Error;
    case 2:  return monaco.MarkerSeverity.Warning;
    case 3:  return monaco.MarkerSeverity.Info;
    case 4:  return monaco.MarkerSeverity.Hint;
    default: return monaco.MarkerSeverity.Warning;
  }
}

function normalizeMarkup(contents: any): Array<{ value: string }> {
  if (!contents) return [];
  if (typeof contents === 'string') return [{ value: contents }];
  if (Array.isArray(contents)) {
    return contents.flatMap((c: any) =>
      typeof c === 'string' ? [{ value: c }] : c?.value ? [{ value: c.value }] : []
    );
  }
  if (contents.value) return [{ value: contents.value }];
  return [];
}

function lspCompletionToMonaco(monaco: any, item: any, range: any) {
  // LSP CompletionItemKind → Monaco CompletionItemKind (both 1-based, mostly aligned)
  const kindMap: Record<number, number> = {
    1:  monaco.languages.CompletionItemKind.Text,
    2:  monaco.languages.CompletionItemKind.Method,
    3:  monaco.languages.CompletionItemKind.Function,
    4:  monaco.languages.CompletionItemKind.Constructor,
    5:  monaco.languages.CompletionItemKind.Field,
    6:  monaco.languages.CompletionItemKind.Variable,
    7:  monaco.languages.CompletionItemKind.Class,
    8:  monaco.languages.CompletionItemKind.Interface,
    9:  monaco.languages.CompletionItemKind.Module,
    10: monaco.languages.CompletionItemKind.Property,
    14: monaco.languages.CompletionItemKind.Keyword,
    17: monaco.languages.CompletionItemKind.File,
    18: monaco.languages.CompletionItemKind.Reference,
  };
  const insertText = item.textEdit?.newText ?? item.insertText ?? item.label ?? '';
  const isSnippet  = item.insertTextFormat === 2; // LSP InsertTextFormat.Snippet
  return {
    label:            item.label ?? '',
    kind:             kindMap[item.kind ?? 1] ?? monaco.languages.CompletionItemKind.Text,
    detail:           item.detail ?? undefined,
    documentation:    item.documentation
                        ? (typeof item.documentation === 'string'
                            ? item.documentation
                            : item.documentation.value ?? '')
                        : undefined,
    insertText,
    insertTextRules:  isSnippet
                        ? monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet
                        : undefined,
    range,
    sortText:         item.sortText ?? item.label ?? '',
    filterText:       item.filterText ?? item.label ?? '',
  };
}
