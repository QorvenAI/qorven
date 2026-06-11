'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useCallback } from 'react';
import { Download, ExternalLink, Loader2, Rocket, CheckCircle2, XCircle, Globe } from 'lucide-react';
import { cn } from '@/lib/utils';
import { getToken, request, BASE as API_BASE } from '@/lib/api-core';
import type { DeployState } from '@/types';

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
  const [deploying, setDeploying] = useState(false);
  const [deployState, setDeployState] = useState<DeployState | null>(null);

  const pollStatus = useCallback(async () => {
    try {
      const res = await request(`/projects/${projectId}/deploy/status`) as DeployState;
      setDeployState(res);
      if (res.status === 'live' || res.status === 'failed' || res.status === 'stopped') {
        setDeploying(false);
      }
    } catch {}
  }, [projectId]);

  useEffect(() => {
    pollStatus();
  }, [pollStatus]);

  useEffect(() => {
    if (!deploying) return;
    const interval = setInterval(pollStatus, 1500);
    return () => clearInterval(interval);
  }, [deploying, pollStatus]);

  async function handleDeploy() {
    setDeploying(true);
    setDownloadError('');
    try {
      const res = await request(`/projects/${projectId}/deploy`, {
        method: 'POST',
      }) as DeployState;
      setDeployState(res);
    } catch (e: any) {
      setDownloadError(e?.message || 'Deploy failed');
      setDeploying(false);
    }
  }

  const repoUrl = githubRepoUrl(githubOwner, githubRepo);

  const targets: DeployTarget[] = [
    {
      id: 'vercel',
      label: 'Vercel',
      description: 'Deploy to Vercel',
      color: 'bg-[var(--connector-vercel)] text-white',
      action: 'link',
    },
    {
      id: 'netlify',
      label: 'Netlify',
      description: 'Deploy to Netlify',
      color: 'bg-[var(--connector-netlify)] text-white',
      action: 'link',
    },
    {
      id: 'pages',
      label: 'GitHub Pages',
      description: 'Enable Pages in repo settings',
      color: 'bg-[var(--connector-github-pages)] text-white',
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
  const isLive = deployState?.status === 'live';
  const isBuilding = deploying || deployState?.status === 'building' || deployState?.status === 'pushing';

  return (
    <div className={cn('space-y-3', className)}>
      {/* One-click deploy to qorven.run */}
      <div className="rounded-lg border border-primary/20 bg-primary/5 p-3 space-y-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Globe className="h-4 w-4 text-primary" />
            <span className="text-xs font-semibold">Deploy to qorven.run</span>
          </div>
          {isLive && deployState?.url && (
            <a href={deployState.url} target="_blank" rel="noopener noreferrer"
              className="text-2xs text-primary hover:underline truncate max-w-[160px]">
              {deployState.url.replace('https://', '')}
            </a>
          )}
        </div>

        {isLive ? (
          <div className="flex items-center gap-2">
            <CheckCircle2 className="h-3.5 w-3.5 text-green-500" />
            <span className="text-xs text-green-600">Live</span>
            <button onClick={handleDeploy}
              className="ml-auto rounded bg-primary/10 px-2 py-0.5 text-2xs text-primary hover:bg-primary/20">
              Redeploy
            </button>
          </div>
        ) : isBuilding ? (
          <div className="space-y-1.5">
            <div className="flex items-center gap-2">
              <Loader2 className="h-3.5 w-3.5 animate-spin text-primary" />
              <span className="text-xs text-muted-foreground capitalize">
                {deployState?.status === 'pushing' ? 'Pushing to edge...' : 'Building container...'}
              </span>
            </div>
            {deployState?.build_log && deployState.build_log.length > 0 && (
              <p className="text-2xs text-muted-foreground font-mono truncate">
                {deployState.build_log[deployState.build_log.length - 1]}
              </p>
            )}
          </div>
        ) : deployState?.status === 'failed' ? (
          <div className="flex items-center gap-2">
            <XCircle className="h-3.5 w-3.5 text-destructive" />
            <span className="text-xs text-destructive truncate">{deployState.error || 'Deploy failed'}</span>
            <button onClick={handleDeploy}
              className="ml-auto rounded bg-primary/10 px-2 py-0.5 text-2xs text-primary hover:bg-primary/20">
              Retry
            </button>
          </div>
        ) : (
          <button onClick={handleDeploy} disabled={deploying}
            className="w-full flex items-center justify-center gap-2 rounded-lg bg-primary py-2 text-xs font-semibold text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50">
            <Rocket className="h-3.5 w-3.5" />
            Deploy Now
          </button>
        )}

        {deployState?.framework && !isLive && !isBuilding && deployState.status !== 'failed' && (
          <p className="text-2xs text-muted-foreground">
            Detected: {deployState.framework} — auto-generates Dockerfile
          </p>
        )}
      </div>

      {/* External deploy targets */}
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
