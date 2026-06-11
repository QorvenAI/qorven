'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useCallback } from 'react';
import { Download, ExternalLink, Loader2, Rocket, CheckCircle2, XCircle, Globe, Bug } from 'lucide-react';
import { cn } from '@/lib/utils';
import { getToken, BASE as API_BASE } from '@/lib/api-core';
import { deployApi } from '@/lib/api-workspace';
import type { DeployState, DeployTargetName } from '@/types';

interface DeployPanelProps {
  projectId: string;
  projectName: string;
  githubOwner?: string;
  githubRepo?: string;
  className?: string;
}

interface ExternalTarget {
  id: string;
  label: string;
  description: string;
  color: string;
  action: 'link' | 'download';
}

const DEPLOY_TARGETS: { value: DeployTargetName; label: string; hint: string }[] = [
  { value: 'local',          label: 'Local',   hint: 'Runs on this machine' },
  { value: 'hosted',         label: 'Hosted',  hint: 'Public preview URL' },
  { value: 'cloud:vercel',   label: 'Vercel',  hint: 'Needs a connected repo + token in Settings' },
  { value: 'cloud:netlify',  label: 'Netlify', hint: 'Needs a connected repo + token in Settings' },
];

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
  const [downloading, setDownloading]         = useState(false);
  const [downloadError, setDownloadError]     = useState('');
  const [deploying, setDeploying]             = useState(false);
  const [deployState, setDeployState]         = useState<DeployState | null>(null);
  const [selectedTarget, setSelectedTarget]   = useState<DeployTargetName>('hosted');

  // Bug-report state
  const [bugOpen, setBugOpen]         = useState(false);
  const [bugTitle, setBugTitle]       = useState('');
  const [bugBody, setBugBody]         = useState('');
  const [bugLoading, setBugLoading]   = useState(false);
  const [bugSuccess, setBugSuccess]   = useState(false);
  const [bugError, setBugError]       = useState('');

  const pollStatus = useCallback(async () => {
    try {
      const res = await deployApi.deployStatus(projectId);
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
      const res = await deployApi.deploy(projectId, selectedTarget);
      setDeployState(res);
    } catch (e: any) {
      setDownloadError(e?.message || 'Deploy failed');
      setDeploying(false);
    }
  }

  async function handleReportBug() {
    if (!bugTitle.trim()) return;
    setBugLoading(true);
    setBugError('');
    try {
      await deployApi.reportBug(projectId, { title: bugTitle.trim(), body: bugBody.trim() });
      setBugSuccess(true);
      setBugTitle('');
      setBugBody('');
    } catch (e: any) {
      setBugError(e?.message || 'Failed to open issue');
    } finally {
      setBugLoading(false);
    }
  }

  const repoUrl = githubRepoUrl(githubOwner, githubRepo);

  const externalTargets: ExternalTarget[] = [
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

  const hasGitHub  = !!repoUrl;
  const isLive     = deployState?.status === 'live';
  const isBuilding = deploying || deployState?.status === 'building' || deployState?.status === 'pushing';

  // Resolve the public URL: prefer deployed_url, fall back to url
  const liveUrl = deployState?.deployed_url || deployState?.url || '';

  return (
    <div className={cn('space-y-3', className)}>
      {/* One-click deploy */}
      <div className="rounded-lg border border-primary/20 bg-primary/5 p-3 space-y-2">
        {/* Header row */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Globe className="h-4 w-4 text-primary" />
            <span className="text-xs font-semibold">Deploy project</span>
          </div>
          {isLive && liveUrl && (
            <a
              href={liveUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="text-2xs text-primary hover:underline truncate max-w-[160px]"
            >
              {liveUrl.replace('https://', '')}
            </a>
          )}
        </div>

        {/* Target picker — segmented control */}
        <div className="flex rounded-md border border-border overflow-hidden">
          {DEPLOY_TARGETS.map(t => (
            <button
              key={t.value}
              type="button"
              title={t.hint}
              onClick={() => setSelectedTarget(t.value)}
              className={cn(
                'flex-1 py-1 text-2xs font-medium transition-colors truncate px-1',
                selectedTarget === t.value
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-background text-muted-foreground hover:bg-muted',
              )}
            >
              {t.label}
            </button>
          ))}
        </div>

        {/* Per-target helper text */}
        <p className="text-2xs text-muted-foreground leading-snug">
          {DEPLOY_TARGETS.find(t => t.value === selectedTarget)?.hint}
        </p>

        {/* Deploy status / action */}
        {isLive ? (
          <div className="space-y-1.5">
            <div className="flex items-center gap-2">
              <CheckCircle2 className="h-3.5 w-3.5 text-green-500 shrink-0" />
              <span className="text-xs text-green-600">Live</span>
              {deployState?.target && (
                <span className="text-2xs text-muted-foreground">via {deployState.target}</span>
              )}
              <button
                onClick={handleDeploy}
                className="ml-auto rounded bg-primary/10 px-2 py-0.5 text-2xs text-primary hover:bg-primary/20"
              >
                Redeploy
              </button>
            </div>
            {liveUrl && (
              <a
                href={liveUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1 text-2xs text-primary hover:underline"
              >
                <ExternalLink className="h-3 w-3 shrink-0" />
                <span className="truncate">{liveUrl}</span>
              </a>
            )}
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
            <button
              onClick={handleDeploy}
              className="ml-auto rounded bg-primary/10 px-2 py-0.5 text-2xs text-primary hover:bg-primary/20"
            >
              Retry
            </button>
          </div>
        ) : (
          <button
            onClick={handleDeploy}
            disabled={deploying}
            className="w-full flex items-center justify-center gap-2 rounded-lg bg-primary py-2 text-xs font-semibold text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
          >
            <Rocket className="h-3.5 w-3.5" />
            Deploy Now
          </button>
        )}

        {deployState?.framework && !isLive && !isBuilding && deployState.status !== 'failed' && (
          <p className="text-2xs text-muted-foreground">
            Detected: {deployState.framework} — auto-generates Dockerfile
          </p>
        )}

        {/* Report a bug — shown once deployed (live) */}
        {isLive && (
          <div className="border-t border-border/50 pt-2 space-y-1.5">
            {bugSuccess ? (
              <p className="text-2xs text-green-600">Issue opened — the team is on it.</p>
            ) : bugOpen ? (
              <div className="space-y-1.5">
                <input
                  type="text"
                  value={bugTitle}
                  onChange={e => setBugTitle(e.target.value)}
                  placeholder="Bug title"
                  className="w-full rounded border border-border bg-background px-2 py-1 text-2xs text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/50"
                />
                <textarea
                  value={bugBody}
                  onChange={e => setBugBody(e.target.value)}
                  placeholder="Describe the bug (optional)"
                  rows={3}
                  className="w-full rounded border border-border bg-background px-2 py-1 text-2xs text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/50 resize-none"
                />
                {bugError && (
                  <p className="rounded px-2 py-1 text-2xs bg-destructive/10 text-destructive">
                    {bugError}
                  </p>
                )}
                <div className="flex gap-1.5">
                  <button
                    onClick={handleReportBug}
                    disabled={bugLoading || !bugTitle.trim()}
                    className="flex items-center gap-1 rounded bg-primary px-2 py-0.5 text-2xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                  >
                    {bugLoading && <Loader2 className="h-3 w-3 animate-spin" />}
                    Submit
                  </button>
                  <button
                    onClick={() => { setBugOpen(false); setBugError(''); setBugTitle(''); setBugBody(''); }}
                    className="rounded px-2 py-0.5 text-2xs text-muted-foreground hover:text-foreground"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            ) : (
              <button
                onClick={() => { setBugOpen(true); setBugSuccess(false); }}
                className="flex items-center gap-1 text-2xs text-muted-foreground hover:text-foreground transition-colors"
              >
                <Bug className="h-3 w-3" />
                Report a bug
              </button>
            )}
          </div>
        )}
      </div>

      {/* External deploy targets */}
      <div className="grid grid-cols-2 gap-2">
        {externalTargets.map(t => {
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
