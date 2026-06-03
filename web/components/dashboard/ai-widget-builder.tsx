'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Loader2, RefreshCw, Sparkles } from 'lucide-react';
import { dashboardLayout } from '@/lib/api-dashboard';
import { WidgetRegistry, type WidgetConfig } from './widget-registry';

interface AIWidgetBuilderProps {
  open: boolean;
  onClose: () => void;
  onAdd: (config: WidgetConfig) => void;
}

export function AIWidgetBuilder({ open, onClose, onAdd }: AIWidgetBuilderProps) {
  const [prompt, setPrompt] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [preview, setPreview] = useState<WidgetConfig | null>(null);

  async function generate() {
    if (!prompt.trim()) return;
    setLoading(true);
    setError(null);
    try {
      const config = await dashboardLayout.generateWidget(prompt.trim());
      setPreview(config);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Generation failed');
    } finally {
      setLoading(false);
    }
  }

  function handleAdd() {
    if (preview) { onAdd(preview); onClose(); setPreview(null); setPrompt(''); }
  }

  function handleClose() {
    onClose();
    setPreview(null);
    setPrompt('');
    setError(null);
  }

  const PreviewWidget = preview ? WidgetRegistry[preview.type] : null;

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) handleClose(); }}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Sparkles className="size-4 text-primary" />
            AI Widget Builder
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-4">
          {/* Prompt input */}
          <div className="space-y-2">
            <label className="text-xs font-medium text-foreground">
              Describe the widget you want
            </label>
            <div className="flex gap-2">
              <input
                type="text"
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter' && !loading) generate(); }}
                placeholder="e.g. Show me total spend today as a big metric with a dollar sign prefix"
                className="flex-1 h-9 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
              <Button
                variant="primary"
                size="sm"
                onClick={generate}
                disabled={loading || !prompt.trim()}
                className="h-9"
              >
                {loading ? <Loader2 className="size-4 animate-spin" /> : 'Generate'}
              </Button>
            </div>
          </div>

          {/* Error */}
          {error && (
            <p className="text-sm text-destructive bg-destructive/10 rounded-lg px-3 py-2">{error}</p>
          )}

          {/* Preview */}
          {preview && (
            <div className="space-y-2">
              <p className="text-xs font-medium text-foreground">Preview</p>
              <div className="rounded-xl border border-border overflow-hidden h-32 bg-card">
                {PreviewWidget ? (
                  <PreviewWidget config={preview} />
                ) : (
                  <div className="h-full flex items-center justify-center text-xs text-muted-foreground">
                    Widget type "{preview.type}" not yet registered
                  </div>
                )}
              </div>
              <div className="text-xs text-muted-foreground space-y-0.5">
                <p><span className="font-medium">Type:</span> {preview.type}</p>
                <p><span className="font-medium">Data source:</span> {preview.dataSource}</p>
                <p><span className="font-medium">Title:</span> {preview.title}</p>
              </div>
            </div>
          )}
        </DialogBody>

        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={handleClose}>Cancel</Button>
          {preview && (
            <Button variant="ghost" size="sm" onClick={generate} disabled={loading} className="gap-1.5">
              <RefreshCw className="size-3.5" />
              Regenerate
            </Button>
          )}
          <Button variant="primary" size="sm" onClick={handleAdd} disabled={!preview}>
            Add to Dashboard
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
