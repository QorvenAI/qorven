'use client';

import { useEffect, useState } from 'react';
import { request } from '@/lib/api-core';
import { PageShell } from '@/components/layouts/page-shell';
import { Shield, CheckCircle, XCircle, AlertTriangle } from 'lucide-react';

interface QualityStats {
  total_outputs: number;
  passed_count: number;
  avg_quality_score: number;
  pass_rate: number;
  top_issues: { rule: string; count: number }[];
}

function StatCard({ icon: Icon, label, value, color }: { icon: any; label: string; value: string; color: string }) {
  return (
    <div className="rounded-lg border border-border bg-card/50 p-4 flex flex-col gap-1">
      <div className="flex items-center gap-2 text-muted-foreground">
        <Icon className={`h-4 w-4 ${color}`} />
        <span className="text-xs">{label}</span>
      </div>
      <span className="text-lg font-semibold text-foreground">{value}</span>
    </div>
  );
}

const RULE_LABELS: Record<string, { label: string; severity: string }> = {
  pii_email: { label: 'PII: Email detected', severity: 'critical' },
  pii_phone: { label: 'PII: Phone number', severity: 'warning' },
  pii_ssn: { label: 'PII: SSN detected', severity: 'critical' },
  pii_credit_card: { label: 'PII: Credit card', severity: 'critical' },
  empty_content: { label: 'Empty output', severity: 'critical' },
  article_min_length: { label: 'Article too short', severity: 'warning' },
  social_max_length: { label: 'Social post too long', severity: 'warning' },
  excessive_length: { label: 'Excessive output length', severity: 'info' },
};

export default function QualityPage() {
  const [stats, setStats] = useState<QualityStats | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    request<QualityStats>('/quality/stats')
      .then(setStats)
      .catch(() => setStats(null))
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <PageShell
        title="Output Quality"
        description="Automated validation scores and compliance monitoring"
        contentClassName="flex items-center justify-center"
      >
        <span className="text-muted-foreground text-sm">Loading...</span>
      </PageShell>
    );
  }

  const total = stats?.total_outputs || 0;
  const passed = stats?.passed_count || 0;
  const avgScore = stats?.avg_quality_score || 0;
  const passRate = stats?.pass_rate || 0;
  const issues = stats?.top_issues || [];

  return (
    <PageShell
      title="Output Quality"
      description="Automated validation scores and compliance monitoring"
      contentClassName="px-6 py-5"
    >
      <div className="space-y-6">
        {/* Stats row */}
        <div className="grid grid-cols-4 gap-4">
          <StatCard icon={Shield} label="Total Outputs" value={total.toString()} color="text-blue-400" />
          <StatCard icon={CheckCircle} label="Auto-Approved" value={passed.toString()} color="text-emerald-400" />
          <StatCard icon={AlertTriangle} label="Avg Score" value={avgScore.toFixed(1)} color="text-amber-400" />
          <StatCard icon={XCircle} label="Pass Rate" value={`${passRate.toFixed(1)}%`} color="text-violet-400" />
        </div>

        {/* Score gauge */}
        <div className="rounded-lg border border-border bg-card/50 p-4">
          <h3 className="text-xs font-medium text-muted-foreground mb-3">Quality Score Distribution</h3>
          <div className="flex items-center gap-2">
            <div className="flex-1 h-3 rounded-full bg-muted/30 overflow-hidden flex">
              <div className="h-full bg-red-500/70" style={{ width: `${Math.max(10 - avgScore, 0) * 10}%` }} title="Below threshold" />
              <div className="h-full bg-amber-500/70" style={{ width: '20%' }} title="Needs review" />
              <div className="h-full bg-emerald-500/70" style={{ width: `${avgScore * 10}%` }} title="Auto-approved" />
            </div>
            <span className="text-xs text-muted-foreground">{avgScore.toFixed(1)}/10</span>
          </div>
          <div className="flex justify-between mt-1 text-2xs text-muted-foreground">
            <span>Blocked (&lt;4.0)</span>
            <span>Review (4-7)</span>
            <span>Auto-approve (&gt;7.0)</span>
          </div>
        </div>

        {/* Top issues */}
        <div className="space-y-2">
          <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Top Validation Issues (30d)</h3>
          <div className="rounded-lg border border-border bg-card/50 divide-y divide-border">
            {issues.length === 0 ? (
              <div className="p-4 text-xs text-muted-foreground">No validation issues recorded</div>
            ) : (
              issues.map((issue, i) => {
                const meta = RULE_LABELS[issue.rule] || { label: issue.rule, severity: 'info' };
                const sevColor = meta.severity === 'critical' ? 'text-red-400' : meta.severity === 'warning' ? 'text-amber-400' : 'text-blue-400';
                return (
                  <div key={i} className="flex items-center justify-between px-4 py-2.5">
                    <div className="flex items-center gap-2">
                      <span className={`text-2xs font-medium uppercase ${sevColor}`}>{meta.severity}</span>
                      <span className="text-xs text-foreground">{meta.label}</span>
                    </div>
                    <span className="text-xs text-muted-foreground">{issue.count} occurrences</span>
                  </div>
                );
              })
            )}
          </div>
        </div>

        {/* Validation rules */}
        <div className="space-y-2">
          <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Active Validation Rules</h3>
          <div className="rounded-lg border border-border bg-card/50 divide-y divide-border">
            {Object.entries(RULE_LABELS).map(([key, meta]) => (
              <div key={key} className="flex items-center justify-between px-4 py-2.5">
                <div className="flex items-center gap-2">
                  <div className={`w-2 h-2 rounded-full ${meta.severity === 'critical' ? 'bg-red-400' : meta.severity === 'warning' ? 'bg-amber-400' : 'bg-blue-400'}`} />
                  <span className="text-xs text-foreground">{meta.label}</span>
                </div>
                <span className="text-2xs text-muted-foreground">{meta.severity}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </PageShell>
  );
}
