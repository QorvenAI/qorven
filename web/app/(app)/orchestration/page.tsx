'use client';

import { useEffect, useState } from 'react';
import { request } from '@/lib/api-core';
import { PageShell } from '@/components/layouts/page-shell';
import { Route, Play, ArrowRight, Trash2, Shield, DollarSign, BarChart3, Workflow } from 'lucide-react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/qor/tabs';

// ─── Types ───────────────────────────────────────────────────────────────────

interface RoutingCondition { field: string; operator: string; value: string }
interface RoutingAction { agent_key: string; priority: number; sla_ms: number; fallback: string }
interface RoutingRule { id: string; name: string; priority: number; conditions: RoutingCondition[]; action: RoutingAction; enabled: boolean }
interface RoutingDecision { agent_key: string; rule_id: string; rule_name: string; method: string; priority: number }
interface SubagentRun { id: string; parent_id: string; agent_key: string; task: string; status: string; depth: number; cost_uusd: number; created_at: string; completed_at: string }
interface OrchRun { id: string; workflow_id: string; status: string; current_step_id: string; total_cost_uusd: number; started_at: string; completed_at: string; error: string }

const PRIORITY_LABELS = ['Interactive', 'Background', 'Batch'];

// ─── Routing Rules Tab ───────────────────────────────────────────────────────

function RoutingRulesTab() {
  const [rules, setRules] = useState<RoutingRule[]>([]);
  const [testContent, setTestContent] = useState('');
  const [testResult, setTestResult] = useState<RoutingDecision | null>(null);
  const [loading, setLoading] = useState(true);

  const loadRules = () => {
    request<{ rules: RoutingRule[] }>('/routing/rules')
      .then(res => setRules(res.rules || []))
      .catch(() => setRules([]))
      .finally(() => setLoading(false));
  };

  useEffect(() => { loadRules(); }, []);

  const handleDelete = async (id: string) => {
    await request(`/routing/rules/${id}`, { method: 'DELETE' });
    loadRules();
  };

  const handleTest = async () => {
    if (!testContent.trim()) return;
    const result = await request<RoutingDecision>('/routing/test', {
      method: 'POST',
      body: JSON.stringify({ content: testContent, channel: 'web' }),
    });
    setTestResult(result);
  };

  return (
    <div className="space-y-5">
      <div className="rounded-lg border border-border bg-card/50 p-4 space-y-3">
        <h3 className="text-xs font-medium text-muted-foreground">Test Intent Routing</h3>
        <div className="flex gap-2">
          <input type="text" value={testContent} onChange={e => setTestContent(e.target.value)}
            placeholder="Type a message to test routing..."
            className="flex-1 px-3 py-1.5 rounded-md border border-border bg-background text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
            onKeyDown={e => e.key === 'Enter' && handleTest()} />
          <button onClick={handleTest} disabled={!testContent.trim()}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-xs font-medium disabled:opacity-50">
            <Play className="h-3 w-3" /> Test
          </button>
        </div>
        {testResult && (
          <div className="flex items-center gap-2 px-3 py-2 rounded-md bg-muted/30 text-xs">
            <ArrowRight className="h-3 w-3 text-emerald-400" />
            <span className="text-foreground font-medium">{testResult.agent_key}</span>
            <span className="text-muted-foreground">via {testResult.method}</span>
            {testResult.rule_name && <span className="text-muted-foreground">(rule: {testResult.rule_name})</span>}
            <span className="ml-auto text-muted-foreground">{PRIORITY_LABELS[testResult.priority] || 'Unknown'}</span>
          </div>
        )}
      </div>

      {loading ? (
        <div className="text-sm text-muted-foreground py-8 text-center">Loading...</div>
      ) : rules.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border p-8 flex flex-col items-center gap-2">
          <Route className="h-8 w-8 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">No intent routing rules configured</p>
          <p className="text-xs text-muted-foreground/70">All messages route via skill-based matching or default to chief</p>
        </div>
      ) : (
        <div className="rounded-lg border border-border bg-card/50 divide-y divide-border">
          {rules.map(rule => (
            <div key={rule.id} className="flex items-center gap-3 px-4 py-3">
              <span className="text-2xs text-muted-foreground w-6 text-center font-mono">{rule.priority}</span>
              <div className="flex-1 min-w-0">
                <div className="text-xs text-foreground font-medium">{rule.name}</div>
                <div className="flex items-center gap-1 mt-0.5 text-2xs text-muted-foreground">
                  {rule.conditions.map((c, i) => (
                    <span key={i}>{i > 0 && ' AND '}{c.field} {c.operator} "{c.value}"</span>
                  ))}
                  <ArrowRight className="h-2.5 w-2.5 mx-1" />
                  <span className="text-emerald-400">{rule.action.agent_key}</span>
                </div>
              </div>
              <button onClick={() => handleDelete(rule.id)} className="p-1 text-muted-foreground hover:text-red-400">
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ─── Subagent Runs Tab ───────────────────────────────────────────────────────

function SubagentRunsTab() {
  const [runs, setRuns] = useState<SubagentRun[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    request<{ runs: SubagentRun[] }>('/audit/subagent-runs')
      .then(res => setRuns(res.runs || []))
      .catch(() => setRuns([]))
      .finally(() => setLoading(false));
  }, []);

  const statusColor = (s: string) => s === 'completed' ? 'text-emerald-400' : s === 'failed' ? 'text-red-400' : 'text-amber-400';

  if (loading) return <div className="text-sm text-muted-foreground py-8 text-center">Loading...</div>;

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{runs.length} subagent runs recorded</p>
      {runs.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
          No subagent runs recorded yet
        </div>
      ) : (
        <div className="rounded-lg border border-border bg-card/50 divide-y divide-border">
          {runs.slice(0, 50).map(run => (
            <div key={run.id} className="px-4 py-2.5 flex items-start gap-3">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className={`text-2xs font-medium ${statusColor(run.status)}`}>{run.status}</span>
                  <span className="text-xs text-foreground truncate">{run.task.substring(0, 80)}{run.task.length > 80 ? '...' : ''}</span>
                </div>
                <div className="flex items-center gap-3 mt-0.5 text-2xs text-muted-foreground">
                  <span>depth: {run.depth}</span>
                  {run.agent_key && <span>agent: {run.agent_key}</span>}
                  <span>parent: {run.parent_id.substring(0, 8)}</span>
                  {run.cost_uusd > 0 && <span>${(run.cost_uusd / 1_000_000).toFixed(4)}</span>}
                </div>
              </div>
              <span className="text-2xs text-muted-foreground whitespace-nowrap">
                {new Date(run.created_at).toLocaleTimeString()}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ─── Workflow Runs Tab ────────────────────────────────────────────────────────

function WorkflowRunsTab() {
  const [runs, setRuns] = useState<OrchRun[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    request<{ runs: OrchRun[] }>('/orchestration/runs')
      .then(res => setRuns(res.runs || []))
      .catch(() => setRuns([]))
      .finally(() => setLoading(false));
  }, []);

  const statusColor = (s: string) => {
    if (s === 'completed') return 'bg-emerald-500/20 text-emerald-400';
    if (s === 'failed') return 'bg-red-500/20 text-red-400';
    if (s === 'running') return 'bg-blue-500/20 text-blue-400';
    return 'bg-zinc-500/20 text-zinc-400';
  };

  if (loading) return <div className="text-sm text-muted-foreground py-8 text-center">Loading...</div>;

  return (
    <div className="space-y-3">
      {runs.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border p-8 text-center">
          <Workflow className="h-8 w-8 text-muted-foreground/50 mx-auto mb-2" />
          <p className="text-sm text-muted-foreground">No orchestration runs yet</p>
          <p className="text-xs text-muted-foreground/70 mt-1">Multi-agent workflow runs will appear here</p>
        </div>
      ) : (
        <div className="rounded-lg border border-border bg-card/50 divide-y divide-border">
          {runs.map(run => (
            <div key={run.id} className="px-4 py-3 flex items-center gap-3">
              <span className={`px-1.5 py-0.5 rounded text-2xs font-medium ${statusColor(run.status)}`}>{run.status}</span>
              <div className="flex-1 min-w-0">
                <span className="text-xs text-foreground">Step: {run.current_step_id || 'done'}</span>
                {run.error && <p className="text-2xs text-red-400 mt-0.5 truncate">{run.error}</p>}
              </div>
              <span className="text-2xs text-muted-foreground">${(run.total_cost_uusd / 1_000_000).toFixed(4)}</span>
              <span className="text-2xs text-muted-foreground">{new Date(run.started_at).toLocaleDateString()}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ─── Main Page ───────────────────────────────────────────────────────────────

export default function OrchestrationPage() {
  return (
    <PageShell
      title="Orchestration"
      description="ERP-grade agent routing, subagent tracking, and multi-agent workflow execution"
      contentClassName="flex flex-col overflow-hidden px-0 py-0 sm:px-0"
    >
      <Tabs defaultValue="routing" className="flex flex-col flex-1 overflow-hidden">
        <div className="px-6 py-3 border-b border-border">
          <TabsList variant="default" size="sm">
            <TabsTrigger value="routing" className="gap-1.5">
              <Route className="h-3.5 w-3.5" /> Intent Rules
            </TabsTrigger>
            <TabsTrigger value="subagents" className="gap-1.5">
              <Shield className="h-3.5 w-3.5" /> Subagent Runs
            </TabsTrigger>
            <TabsTrigger value="workflows" className="gap-1.5">
              <Workflow className="h-3.5 w-3.5" /> Workflow Runs
            </TabsTrigger>
          </TabsList>
        </div>
        <div className="flex-1 overflow-y-auto px-6 py-5">
          <TabsContent value="routing" className="mt-0"><RoutingRulesTab /></TabsContent>
          <TabsContent value="subagents" className="mt-0"><SubagentRunsTab /></TabsContent>
          <TabsContent value="workflows" className="mt-0"><WorkflowRunsTab /></TabsContent>
        </div>
      </Tabs>
    </PageShell>
  );
}
