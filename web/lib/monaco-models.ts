// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import type * as Monaco from 'monaco-editor';

let _m: typeof Monaco | null = null;

export function setMonaco(m: typeof Monaco) {
  _m = m;
}

/**
 * getOrCreateModel returns a stable ITextModel keyed by the file path's URI, so
 * switching tabs reuses the same model (preserving undo/cursor/scroll) instead
 * of recreating it.
 */
export function getOrCreateModel(
  path: string,
  content: string,
  language: string,
): Monaco.editor.ITextModel | null {
  if (!_m) return null;
  const uri = _m.Uri.file(path);
  const existing = _m.editor.getModel(uri);
  if (existing) return existing;
  return _m.editor.createModel(content, language, uri);
}

/**
 * syncModelContent re-syncs a model's content WITHOUT destroying the undo stack
 * (uses pushEditOperations, never setValue) — for when the agent writes a file.
 */
export function syncModelContent(path: string, content: string) {
  if (!_m) return;
  const model = _m.editor.getModel(_m.Uri.file(path));
  if (model && model.getValue() !== content) {
    model.pushEditOperations(
      [],
      [{ range: model.getFullModelRange(), text: content }],
      () => null,
    );
  }
}

export function disposeModel(path: string) {
  if (!_m) return;
  _m.editor.getModel(_m.Uri.file(path))?.dispose();
}
