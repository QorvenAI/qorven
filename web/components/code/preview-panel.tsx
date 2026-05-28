'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState, useCallback } from 'react';
import { Monitor, Tablet, Smartphone, RotateCw, ExternalLink, AlertTriangle, Play, Square } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { request } from '@/lib/api-core';

type DeviceSize = 'desktop' | 'tablet' | 'mobile';

const DEVICE_CONFIG: Record<DeviceSize, { label: string; icon: React.ComponentType<{ className?: string }>; width: number | '100%' }> = {
  desktop: { label: 'Desktop', icon: Monitor,    width: '100%' },
  tablet:  { label: 'Tablet',  icon: Tablet,     width: 768 },
  mobile:  { label: 'Mobile',  icon: Smartphone, width: 390 },
};

interface PreviewPanelProps {
  url: string;
  projectId?: string;
  className?: string;
}

export function PreviewPanel({ url, projectId, className }: PreviewPanelProps) {
  const [device, setDevice] = useState<DeviceSize>('desktop');
  const [frameKey, setFrameKey] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [starting, setStarting] = useState(false);
  const [previewUrl, setPreviewUrl] = useState(url);
  const [serverRunning, setServerRunning] = useState(false);
  const iframeRef = useRef<HTMLIFrameElement>(null);

  const cfg = DEVICE_CONFIG[device];

  useEffect(() => {
    setPreviewUrl(url);
  }, [url]);

  useEffect(() => {
    setLoading(true);
    setError(null);
  }, [previewUrl, frameKey]);

  const startDevServer = useCallback(async () => {
    if (!projectId) return;
    setStarting(true);
    setError(null);
    try {
      const res = await request(`/projects/${projectId}/preview/start`, { method: 'POST' }) as { preview_url?: string };
      if (res?.preview_url) {
        setPreviewUrl(res.preview_url);
        setServerRunning(true);
      }
    } catch (e: any) {
      setError(e.message || 'Failed to start dev server');
    } finally {
      setStarting(false);
    }
  }, [projectId]);

  const stopDevServer = useCallback(async () => {
    if (!projectId) return;
    try {
      await request(`/projects/${projectId}/preview/stop`, { method: 'POST' });
      setServerRunning(false);
      setPreviewUrl('');
    } catch (e: any) {
      setError(e.message || 'Failed to stop dev server');
    }
  }, [projectId]);

  if (!previewUrl) {
    return (
      <div className={cn('flex h-full items-center justify-center', className)}>
        <div className="text-center space-y-3">
          <Monitor className="mx-auto h-10 w-10 text-muted-foreground/30" />
          <p className="text-xs text-muted-foreground">No preview available yet</p>
          {projectId && (
            <Button
              size="sm"
              variant="outline"
              onClick={startDevServer}
              disabled={starting}
              className="gap-1.5"
            >
              <Play className="h-3 w-3" />
              {starting ? 'Starting...' : 'Start Dev Server'}
            </Button>
          )}
          {!projectId && (
            <p className="text-2xs text-muted-foreground/50">Build something first</p>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className={cn('flex h-full flex-col overflow-hidden', className)}>
      {/* Toolbar */}
      <div className="flex shrink-0 items-center gap-2 border-b border-border bg-muted/20 px-3 py-1.5">
        {/* Device selector */}
        <div className="flex items-center rounded-lg border border-border bg-background p-0.5 gap-0.5">
          {(Object.keys(DEVICE_CONFIG) as DeviceSize[]).map(d => {
            const { icon: Icon, label } = DEVICE_CONFIG[d];
            return (
              <button
                key={d}
                type="button"
                title={label}
                onClick={() => setDevice(d)}
                className={cn(
                  'flex items-center justify-center rounded-md p-1.5 transition-all',
                  device === d
                    ? 'bg-muted text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground',
                )}
              >
                <Icon className="h-3.5 w-3.5" />
              </button>
            );
          })}
        </div>

        {/* URL pill */}
        <div className="flex-1 overflow-hidden rounded-md border border-border bg-background px-2 py-1">
          <span className="block truncate font-mono text-2xs text-muted-foreground">{url}</span>
        </div>

        {/* Actions */}
        <Button
          variant="ghost"
          size="icon"
          title="Refresh"
          onClick={() => setFrameKey(k => k + 1)}
          className="h-7 w-7"
        >
          <RotateCw className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          title="Open in new tab"
          onClick={() => window.open(previewUrl, '_blank')}
          className="h-7 w-7"
        >
          <ExternalLink className="h-3.5 w-3.5" />
        </Button>
        {serverRunning && (
          <Button
            variant="ghost"
            size="icon"
            title="Stop dev server"
            onClick={stopDevServer}
            className="h-7 w-7 text-destructive hover:text-destructive"
          >
            <Square className="h-3.5 w-3.5" />
          </Button>
        )}
      </div>

      {/* Preview frame */}
      <div className="relative flex flex-1 items-start justify-center overflow-auto bg-muted/30 p-2">
        <div
          className="relative h-full overflow-hidden rounded-lg border border-border bg-white shadow-lg"
          style={{ width: cfg.width === '100%' ? '100%' : cfg.width, minHeight: '100%' }}
        >
          {loading && (
            <div className="absolute inset-0 flex items-center justify-center bg-background/80 z-10">
              <div className="h-5 w-5 rounded-full border-2 border-primary border-t-transparent animate-spin" />
            </div>
          )}
          <iframe
            ref={iframeRef}
            key={frameKey}
            src={previewUrl}
            sandbox="allow-scripts allow-forms allow-popups allow-same-origin"
            className="h-full w-full border-0"
            onLoad={() => setLoading(false)}
            onError={() => { setLoading(false); setError('Failed to load preview'); }}
            title="App preview"
          />
        </div>
      </div>

      {/* Error banner */}
      {error && (
        <div className="flex shrink-0 items-center gap-2 border-t border-destructive/30 bg-destructive/10 px-3 py-2">
          <AlertTriangle className="h-3.5 w-3.5 shrink-0 text-destructive" />
          <span className="text-xs text-destructive">{error}</span>
          <button
            type="button"
            onClick={() => setError(null)}
            className="ml-auto text-xs text-destructive/70 hover:text-destructive"
          >
            Dismiss
          </button>
        </div>
      )}
    </div>
  );
}
