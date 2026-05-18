'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useCallback } from 'react';
import { Upload } from 'lucide-react';
import { cn } from '@/lib/utils';

interface Props {
  agentId: string;
  onUploaded?: () => void;
  onFilesDropped?: (files: File[]) => void;
  children: React.ReactNode;
  className?: string;
}

export function UploadZone({ agentId, onUploaded, onFilesDropped, children, className }: Props) {
  const [dragging, setDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

  const upload = useCallback(async (files: FileList) => {
    const fileArray = Array.from(files);

    // Notify parent about dropped files (for chat attachment)
    if (onFilesDropped) {
      onFilesDropped(fileArray);
    }

    // Upload to drive
    setUploading(true);
    for (const file of fileArray) {
      const form = new FormData();
      form.append('file', file);
      form.append('agent_id', agentId);
      await fetch('/api/v1/drive/upload', { method: 'POST', headers: { Authorization: `Bearer ${getToken()}` }, body: form }).catch(() => {});
    }
    setUploading(false);
    onUploaded?.();
  }, [agentId, getToken(), onUploaded, onFilesDropped]);

  return (
    <div className={cn('relative', className)}
      onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => { e.preventDefault(); setDragging(false); if (e.dataTransfer.files.length) upload(e.dataTransfer.files); }}>
      {children}
      {(dragging || uploading) && (
        <div className="absolute inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm rounded-xl border-2 border-dashed border-primary">
          <div className="text-center">
            <Upload className="h-8 w-8 mx-auto text-primary mb-2" />
            <p className="text-sm font-medium">{uploading ? 'Uploading...' : 'Drop files here'}</p>
            <p className="text-xs text-muted-foreground mt-1">Files will be attached to this conversation</p>
          </div>
        </div>
      )}
    </div>
  );
}
