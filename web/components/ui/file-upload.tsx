'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import * as React from 'react';
import { cn } from '@/lib/utils';
import { Upload, X, FileIcon } from 'lucide-react';

interface FileUploadProps {
  accept?: string;
  multiple?: boolean;
  maxSize?: number; // bytes
  onFiles?: (files: File[]) => void;
  className?: string;
  disabled?: boolean;
}

export function FileUpload({ accept, multiple, maxSize = 10 * 1024 * 1024, onFiles, className, disabled }: FileUploadProps) {
  const [files, setFiles] = React.useState<File[]>([]);
  const [dragOver, setDragOver] = React.useState(false);
  const inputRef = React.useRef<HTMLInputElement>(null);

  const handleFiles = (newFiles: FileList | null) => {
    if (!newFiles) return;
    const valid = Array.from(newFiles).filter(f => f.size <= maxSize);
    const updated = multiple ? [...files, ...valid] : valid.slice(0, 1);
    setFiles(updated);
    onFiles?.(updated);
  };

  const removeFile = (index: number) => {
    const updated = files.filter((_, i) => i !== index);
    setFiles(updated);
    onFiles?.(updated);
  };

  return (
    <div className={cn('space-y-2', className)}>
      <div
        className={cn(
          'relative flex flex-col items-center justify-center rounded-xl border-2 border-dashed p-6 transition-colors cursor-pointer',
          dragOver ? 'border-primary bg-primary/5' : 'border-border hover:border-primary/50',
          disabled && 'opacity-50 pointer-events-none',
        )}
        onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
        onDragLeave={() => setDragOver(false)}
        onDrop={(e) => { e.preventDefault(); setDragOver(false); handleFiles(e.dataTransfer.files); }}
        onClick={() => inputRef.current?.click()}
      >
        <Upload className="h-8 w-8 text-muted-foreground mb-2" />
        <p className="text-sm font-medium">Drop files here or click to browse</p>
        <p className="text-2xs text-muted-foreground mt-1">
          {accept ? `Accepted: ${accept}` : 'Any file type'} · Max {(maxSize / 1024 / 1024).toFixed(0)}MB
        </p>
        <input ref={inputRef} type="file" accept={accept} multiple={multiple} className="hidden"
          onChange={(e) => handleFiles(e.target.files)} />
      </div>

      {files.length > 0 && (
        <div className="space-y-1">
          {files.map((file, i) => (
            <div key={i} className="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2">
              <FileIcon className="h-4 w-4 text-muted-foreground shrink-0" />
              <span className="text-sm truncate flex-1">{file.name}</span>
              <span className="text-2xs text-muted-foreground">{(file.size / 1024).toFixed(0)}KB</span>
              <button onClick={(e) => { e.stopPropagation(); removeFile(i); }}
                className="text-muted-foreground hover:text-destructive">
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
