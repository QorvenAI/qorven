'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import { Monitor, Tablet, Smartphone, RotateCw, ExternalLink, AlertTriangle } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';

type DeviceSize = 'desktop' | 'tablet' | 'mobile';

const DEVICE_CONFIG: Record<DeviceSize, { label: string; icon: React.ComponentType<{ className?: string }>; width: number | '100%' }> = {
  desktop: { label: 'Desktop', icon: Monitor,    width: '100%' },
  tablet:  { label: 'Tablet',  icon: Tablet,     width: 768 },
  mobile:  { label: 'Mobile',  icon: Smartphone, width: 390 },
};

interface PreviewPanelProps {
  url: string;
  className?: string;
}

export function PreviewPanel({ url, className }: PreviewPanelProps) {
  const [device, setDevice] = useState<DeviceSize>('desktop');
  const [frameKey, setFrameKey] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const iframeRef = useRef<HTMLIFrameElement>(null);

  const cfg = DEVICE_CONFIG[device];

  useEffect(() => {
    setLoading(true);
    setError(null);
  }, [url, frameKey]);

  if (!url) {
    return (
      <div className={cn('flex h-full items-center justify-center', className)}>
        <div className="text-center space-y-2">
          <Monitor className="mx-auto h-10 w-10 text-muted-foreground/30" />
          <p className="text-xs text-muted-foreground">No preview available yet</p>
          <p className="text-2xs text-muted-foreground/50">Build something first</p>
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
          onClick={() => window.open(url, '_blank')}
          className="h-7 w-7"
        >
          <ExternalLink className="h-3.5 w-3.5" />
        </Button>
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
            src={url}
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
