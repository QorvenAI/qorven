'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { Download, ExternalLink, Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { getToken, BASE as API_BASE } from '@/lib/api-core';

interface DeployPanelProps {
  projectId: string;
  projectName: string;
  githubOwner?: string;
  githubRepo?: string;
  className?: string;
}

interface DeployTarget {
  id: string;
  label: string;
  description: string;
  color: string;
  action: 'link' | 'download';
}

function githubRepoUrl(owner?: string, repo?: string) {
  if (!owner || !repo) return '';
  return `https://github.com/${owner}/${repo}`;
}

export function DeployPanel({
  projectId,
  projectName,
  githubOwner,
  githubRepo,
  className,
}: DeployPanelProps) {
  const [downloading, setDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState('');

  const repoUrl = githubRepoUrl(githubOwner, githubRepo);

  const targets: DeployTarget[] = [
    {
      id: 'vercel',
      label: 'Vercel',
      description: 'Deploy to Vercel',
      color: 'bg-black text-white',
      action: 'link',
    },
    {
      id: 'netlify',
      label: 'Netlify',
      description: 'Deploy to Netlify',
      color: 'bg-[#00AD9F] text-white',
      action: 'link',
    },
    {
      id: 'pages',
      label: 'GitHub Pages',
      description: 'Enable Pages in repo settings',
      color: 'bg-[#24292e] text-white',
      action: 'link',
    },
    {
      id: 'zip',
      label: 'Download ZIP',
      description: 'Export all project files',
      color: 'bg-muted text-foreground',
      action: 'download',
    },
  ];

  function handleLink(id: string) {
    if (!repoUrl && id !== 'zip') return;
    let url = '';
    switch (id) {
      case 'vercel':
        url = `https://vercel.com/new/import?s=${encodeURIComponent(repoUrl)}`;
        break;
      case 'netlify':
        url = `https://app.netlify.com/start/deploy?repository=${encodeURIComponent(repoUrl)}`;
        break;
      case 'pages':
        url = `${repoUrl}/settings/pages`;
        break;
    }
    if (url) window.open(url, '_blank', 'noopener,noreferrer');
  }

  async function handleDownload() {
    setDownloading(true);
    setDownloadError('');
    try {
      const res = await fetch(`${API_BASE}/projects/${projectId}/archive`, {
        headers: { Authorization: `Bearer ${getToken()}` },
      });
      if (!res.ok) {
        setDownloadError(`Archive failed (${res.status})`);
        return;
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${projectName}.zip`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e: any) {
      setDownloadError(e?.message || 'Download failed');
    } finally {
      setDownloading(false);
    }
  }

  const hasGitHub = !!repoUrl;

  return (
    <div className={cn('space-y-3', className)}>
      <div className="grid grid-cols-2 gap-2">
        {targets.map(t => {
          const disabled = t.id !== 'zip' && !hasGitHub;
          return (
            <button
              key={t.id}
              type="button"
              disabled={disabled || (t.id === 'zip' && downloading)}
              title={disabled ? 'Connect a GitHub repo first' : t.description}
              onClick={() => {
                if (t.action === 'download') handleDownload();
                else handleLink(t.id);
              }}
              className={cn(
                'flex items-center gap-2 rounded-lg border border-border px-3 py-2.5 text-xs font-medium transition-colors',
                'hover:opacity-80 disabled:opacity-40 disabled:cursor-not-allowed',
                t.color,
              )}
            >
              {t.id === 'zip' ? (
                downloading
                  ? <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" />
                  : <Download className="h-3.5 w-3.5 shrink-0" />
              ) : (
                <ExternalLink className="h-3.5 w-3.5 shrink-0" />
              )}
              <span className="truncate">{t.label}</span>
            </button>
          );
        })}
      </div>

      {downloadError && (
        <p className="text-2xs text-destructive">{downloadError}</p>
      )}

      {!hasGitHub && (
        <p className="text-2xs text-muted-foreground">
          Connect a GitHub repo via the GitHub tab to enable Vercel, Netlify, and Pages deploys.
        </p>
      )}
    </div>
  );
}
