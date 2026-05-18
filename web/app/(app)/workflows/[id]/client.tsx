'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * /workflows/[id] — DAG viewer + run history + source (T3.4).
 *
 * Three tabs under one header:
 *   DAG      — @xyflow/react render of the workflow's steps. Nodes
 *              are typed (prompt/tool/condition/…) with a color-coded
 *              left border. Edges come from each step's `next`
 *              pointer + any `branches` map.
 *   History  — recent runs (status + started + duration + error).
 *   Source   — read-only JSON dump of the workflow definition.
 *
 * Drag-to-edit the graph itself is out of scope for this first pass;
 * the read-only viz is the high-value visual win. Edit-via-JSON
 * lands in a later session once we have the PUT endpoint wrapper.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  ArrowLeft, Play, AlertCircle, CheckCircle2, XCircle, Clock,
  RefreshCw, Loader2, Code2, Share2, Save, Plus, Trash2,
  MessageSquare, Wrench, GitBranch, ClipboardList, Globe, Users,
  Bell, Pause, Power, Settings2,
} from 'lucide-react';
import {
  ReactFlow, Background, Controls, MiniMap, Handle, Position,
  useNodesState, useEdgesState, addEdge,
  type Node as FlowNode, type Edge as FlowEdge, type Connection, type OnConnect,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { ErrorBoundary } from '@/components/error-boundary';
import {
  workflows, parseWorkflowSteps,
  type Workflow, type WorkflowStep, type WorkflowStepType, type WorkflowRun,
} from '@/lib/api';
import { cn } from '@/lib/utils';

const TYPE_META: Record<WorkflowStepType, { icon: typeof Play; color: string; label: string }> = {
  prompt:    { icon: MessageSquare, color: '#8b5cf6', label: 'Prompt' },
  tool:      { icon: Wrench,        color: '#06b6d4', label: 'Tool' },
  condition: { icon: GitBranch,     color: '#f59e0b', label: 'Condition' },
  collect:   { icon: ClipboardList, color: '#10b981', label: 'Collect' },
  api:       { icon: Globe,         color: '#ef4444', label: 'API' },
  delegate:  { icon: Users,         color: '#a855f7', label: 'Delegate' },
  notify:    { icon: Bell,          color: '#ec4899', label: 'Notify' },
  wait:      { icon: Pause,         color: '#64748b', label: 'Wait' },
};
const typeMeta = (t: string) =>
  TYPE_META[t as WorkflowStepType] ?? { icon: Code2, color: '#64748b', label: t };

type Tab = 'dag' | 'history' | 'source' | 'settings';

export default function WorkflowDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [wf, setWf] = useState<Workflow | null>(null);
  const [runs, setRuns] = useState<WorkflowRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>('dag');
  const [triggering, setTriggering] = useState(false);
  const [toggling, setToggling] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    workflows.get(id)
      .then((d) => setWf(d))
      .catch((e) => setError(e instanceof Error ? e.message : 'Failed to load'))
      .finally(() => setLoading(false));
    // Runs are a secondary concern; failure shouldn't surface as page error.
    workflows.runs(id)
      .then((r) => setRuns(Array.isArray(r) ? r : []))
      .catch(() => setRuns([]));
  }, [id]);

  useEffect(() => { load(); }, [load]);

  const triggerRun = async () => {
    setTriggering(true);
    try {
      await workflows.run(id);
      // Give the backend a beat to register the run row, then refetch.
      setTimeout(load, 600);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Run failed');
    } finally {
      setTriggering(false);
    }
  };

  const toggleEnabled = async () => {
    if (!wf) return;
    setToggling(true);
    try {
      await workflows.update(id, { enabled: !wf.enabled });
      setWf((prev) => prev ? { ...prev, enabled: !prev.enabled } : prev);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Toggle failed');
    } finally {
      setToggling(false);
    }
  };

  const steps = useMemo(() => (wf ? parseWorkflowSteps(wf.steps) : []), [wf]);

  if (loading) {
    return (
      <div className="space-y-4 p-4 lg:p-6">
        <div className="h-8 w-48 animate-pulse rounded bg-muted" />
        <div className="h-[500px] animate-pulse rounded-xl bg-muted/40" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center py-16 text-center">
        <AlertCircle className="h-8 w-8 text-destructive" />
        <p className="mt-2 text-sm text-destructive">{error}</p>
        <button onClick={load} className="mt-3 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">
          Retry
        </button>
      </div>
    );
  }

  if (!wf) return <div className="py-16 text-center text-muted-foreground">Workflow not found</div>;

  return (
    <ErrorBoundary>
      <div className="full-bleed flex h-[calc(100vh-var(--header-height)-var(--status-bar-height,24px))] flex-col gap-3 p-4 lg:p-6">
        {/* Header */}
        <header className="flex items-center gap-3">
          <Link
            href="/workflows"
            className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-xl font-semibold tracking-tight">{wf.name || '(unnamed)'}</h1>
            {wf.description && (
              <p className="truncate text-xs text-muted-foreground">{wf.description}</p>
            )}
          </div>
          <button
            onClick={toggleEnabled}
            disabled={toggling}
            title={wf.enabled ? 'Click to disable' : 'Click to enable'}
            className={cn(
              'inline-flex shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 font-mono text-2xs transition-colors disabled:opacity-50',
              wf.enabled
                ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20'
                : 'border-border bg-muted/40 text-muted-foreground hover:bg-accent',
            )}
          >
            {toggling
              ? <Loader2 className="h-3 w-3 animate-spin" />
              : <Power className="h-3 w-3" />}
            {wf.enabled ? 'enabled' : 'disabled'}
          </button>
          {wf.trigger_type && (
            <span className="shrink-0 rounded-sm bg-muted px-1.5 py-0.5 font-mono text-2xs text-muted-foreground">
              trigger: {wf.trigger_type}
            </span>
          )}
          <button
            onClick={load}
            className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-accent"
          >
            <RefreshCw className="h-3.5 w-3.5" />
            Refresh
          </button>
          <button
            onClick={triggerRun}
            disabled={triggering}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {triggering ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
            Run
          </button>
        </header>

        {/* Tab bar */}
        <div className="-mt-1 flex items-center gap-0 border-b border-border">
          {([
            { id: 'dag',      icon: Share2,    label: `DAG (${steps.length})` },
            { id: 'history',  icon: Clock,     label: `Runs (${runs.length})` },
            { id: 'source',   icon: Code2,     label: 'Source' },
            { id: 'settings', icon: Settings2, label: 'Settings' },
          ] as const).map((t) => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={cn(
                '-mb-px flex items-center gap-1.5 border-b-2 px-3 py-2 text-xs font-medium transition-colors',
                tab === t.id
                  ? 'border-primary text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground',
              )}
            >
              <t.icon className="h-3.5 w-3.5" />
              {t.label}
            </button>
          ))}
        </div>

        {tab === 'dag' && (
          <EditorView
            wfId={id}
            steps={steps}
            onSaved={load}
          />
        )}
        {tab === 'history' && <HistoryView runs={runs} />}
        {tab === 'source' && <SourceView wf={wf} />}
        {tab === 'settings' && <SettingsView wf={wf} onSaved={load} />}
      </div>
    </ErrorBoundary>
  );
}

// ─── Editor view ───────────────────────────────────────────────────
//
// Editable canvas. Drag nodes to reposition, connect output
// handles to input handles to wire `next`, click a node to edit its
// fields in the right rail, Delete/Backspace to remove. The palette
// (left rail) adds a node of any type at a fresh position.
//
// Graph state lives in ReactFlow's nodes/edges arrays; `reconstruct()`
// walks them back into a typed WorkflowStep[] at save time. Positions
// are ephemeral (computed from scratch via computeLayout on load)
// because the current workflow.Workflow schema doesn't persist them.
// If layout churn becomes a UX problem we can add a `layout` column
// and round-trip it — cheap enough to defer.

const NODE_TYPES_ALL: WorkflowStepType[] = [
  'prompt', 'tool', 'condition', 'collect', 'api', 'delegate', 'notify', 'wait',
];

function EditorView({ wfId, steps: initialSteps, onSaved }: {
  wfId: string;
  steps: WorkflowStep[];
  onSaved: () => void;
}) {
  const initial = useMemo(() => buildGraph(initialSteps), [initialSteps]);
  const [nodes, setNodes, onNodesChange] = useNodesState<FlowNode>(initial.nodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState<FlowEdge>(initial.edges);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const initialKeyRef = useRef<string>('');

  // Reset whenever the upstream steps change (e.g. a save-then-reload).
  useEffect(() => {
    const fresh = buildGraph(initialSteps);
    setNodes(fresh.nodes);
    setEdges(fresh.edges);
    initialKeyRef.current = serializeKey(fresh.nodes, fresh.edges);
    setDirty(false);
    setSelectedId(null);
  }, [initialSteps, setNodes, setEdges]);

  // Any local graph change flips dirty — compare serialized shape so we
  // don't mark dirty for benign position tweaks that happen on mount.
  useEffect(() => {
    if (!initialKeyRef.current) return;
    const nowKey = serializeKey(nodes, edges);
    setDirty(nowKey !== initialKeyRef.current);
  }, [nodes, edges]);

  const onConnect: OnConnect = useCallback((params: Connection) => {
    setEdges((eds) => addEdge({
      ...params,
      type: 'smoothstep',
      style: { stroke: '#71717a', strokeWidth: 1.5 },
    }, eds));
  }, [setEdges]);

  const addStep = useCallback((type: WorkflowStepType) => {
    const id = genStepId(type, nodes);
    const meta = typeMeta(type);
    const seed: WorkflowStep = { id, type };
    if (type === 'condition') seed.branches = {};
    if (type === 'collect') seed.fields = [];
    // Drop new nodes in a column to the right of everything current.
    const maxX = nodes.reduce((m, n) => Math.max(m, n.position.x), 0);
    const newNode: FlowNode = {
      id,
      type: 'step',
      position: { x: nodes.length === 0 ? 0 : maxX + 280, y: (nodes.length % 4) * 120 },
      data: { step: seed, color: meta.color, label: meta.label, icon: meta.icon } as StepNodeData,
    };
    setNodes((ns) => [...ns, newNode]);
    setSelectedId(id);
  }, [nodes, setNodes]);

  const updateStep = useCallback((id: string, patch: Partial<WorkflowStep>) => {
    setNodes((ns) => ns.map((n) => {
      if (n.id !== id) return n;
      const data = n.data as StepNodeData;
      const nextStep = { ...data.step, ...patch };
      // Type change replaces the color + icon too.
      if (patch.type && patch.type !== data.step.type) {
        const m = typeMeta(patch.type);
        return { ...n, data: { step: nextStep, color: m.color, label: m.label, icon: m.icon } };
      }
      return { ...n, data: { ...data, step: nextStep } };
    }));
  }, [setNodes]);

  const deleteSelected = useCallback(() => {
    if (!selectedId) return;
    setNodes((ns) => ns.filter((n) => n.id !== selectedId));
    setEdges((es) => es.filter((e) => e.source !== selectedId && e.target !== selectedId));
    setSelectedId(null);
  }, [selectedId, setNodes, setEdges]);

  const save = useCallback(async () => {
    setSaving(true);
    setSaveError(null);
    try {
      const rebuilt = reconstruct(nodes, edges);
      await workflows.update(wfId, { steps: rebuilt });
      setDirty(false);
      onSaved();
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  }, [nodes, edges, wfId, onSaved]);

  const selectedStep = useMemo(() => {
    if (!selectedId) return null;
    const n = nodes.find((x) => x.id === selectedId);
    return n ? (n.data as StepNodeData).step : null;
  }, [selectedId, nodes]);

  return (
    <div className="flex flex-1 gap-3 overflow-hidden">
      {/* Palette */}
      <aside className="flex w-44 shrink-0 flex-col gap-1 rounded-xl border border-border bg-card/40 p-2">
        <p className="px-2 pt-1 pb-1.5 text-2xs uppercase tracking-wider text-muted-foreground">Add step</p>
        {NODE_TYPES_ALL.map((t) => {
          const m = typeMeta(t);
          const Icon = m.icon;
          return (
            <button
              key={t}
              onClick={() => addStep(t)}
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs text-foreground/80 hover:bg-accent"
            >
              <Icon className="h-3.5 w-3.5 shrink-0" style={{ color: m.color }} />
              <span>{m.label}</span>
              <Plus className="ml-auto h-3 w-3 text-muted-foreground" />
            </button>
          );
        })}
        <div className="mt-auto space-y-1 px-1">
          {saveError && <p className="text-2xs text-destructive">{saveError}</p>}
          <button
            onClick={save}
            disabled={!dirty || saving}
            className={cn(
              'flex w-full items-center justify-center gap-1.5 rounded-md px-2 py-2 text-xs font-medium transition-colors',
              dirty
                ? 'bg-primary text-primary-foreground hover:bg-primary/90'
                : 'bg-muted text-muted-foreground',
              saving && 'opacity-60',
            )}
          >
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
            {dirty ? 'Save changes' : 'Saved'}
          </button>
        </div>
      </aside>

      {/* Canvas */}
      <div className="relative flex-1 overflow-hidden rounded-xl border border-border bg-card/40">
        {nodes.length === 0 && (
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
            <div className="text-center">
              <Share2 className="mx-auto h-8 w-8 text-muted-foreground/60" />
              <p className="mt-2 text-sm text-muted-foreground">Empty workflow — add a step to begin.</p>
            </div>
          </div>
        )}
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onNodeClick={(_, n) => setSelectedId(n.id)}
          onPaneClick={() => setSelectedId(null)}
          nodeTypes={{ step: StepNode }}
          fitView
          fitViewOptions={{ padding: 0.2, maxZoom: 1.2 }}
          deleteKeyCode={['Delete', 'Backspace']}
          proOptions={{ hideAttribution: true }}
        >
          <Background color="#52525b" gap={20} size={1} />
          <Controls showInteractive={false} />
          <MiniMap
            zoomable
            pannable
            nodeColor={(n) => (n.data as StepNodeData)?.color ?? '#64748b'}
            maskColor="rgba(0,0,0,0.6)"
          />
        </ReactFlow>
      </div>

      {/* Step editor rail */}
      {selectedStep && (
        <StepEditor
          step={selectedStep}
          allIds={nodes.map((n) => n.id)}
          onChange={(patch) => updateStep(selectedStep.id, patch)}
          onDelete={deleteSelected}
          onClose={() => setSelectedId(null)}
        />
      )}
    </div>
  );
}

interface StepNodeData extends Record<string, unknown> {
  step: WorkflowStep;
  color: string;
  label: string;
  icon: typeof Play;
}

function StepNode({ data, selected }: { data: StepNodeData; selected: boolean }) {
  const Icon = data.icon;
  const s = data.step;
  const caption =
    s.type === 'prompt'    ? (s.prompt ?? '').slice(0, 60)
    : s.type === 'tool'      ? s.tool ?? ''
    : s.type === 'api'       ? `${s.method ?? 'GET'} ${s.url ?? ''}`
    : s.type === 'delegate'  ? (s.soul_key ?? '') + (s.task ? ' · ' + s.task.slice(0, 30) : '')
    : s.type === 'collect'   ? (s.fields ?? []).join(', ')
    : s.type === 'condition' ? Object.keys(s.branches ?? {}).join(' | ') + ' branches'
    : s.type === 'wait'      ? JSON.stringify(s.args ?? {})
    : s.type === 'notify'    ? (s.args?.message as string | undefined ?? '')
    : '';

  return (
    <div
      className={cn(
        'min-w-[200px] max-w-[260px] rounded-md border bg-background shadow-sm',
        selected ? 'border-primary ring-2 ring-primary/30' : 'border-border',
      )}
      style={{ borderLeft: `4px solid ${data.color}` }}
    >
      <Handle type="target" position={Position.Left} style={{ background: data.color, width: 8, height: 8 }} />
      <div className="flex items-center gap-1.5 border-b border-border/60 px-2.5 py-1.5">
        <Icon className="h-3.5 w-3.5" style={{ color: data.color }} />
        <span className="font-mono text-2xs uppercase tracking-wider text-muted-foreground">
          {data.label}
        </span>
        <span className="ml-auto font-mono text-2xs text-muted-foreground">{s.id}</span>
      </div>
      {caption && (
        <div className="line-clamp-2 px-2.5 py-1.5 text-xs text-foreground/85">
          {caption}
        </div>
      )}
      {s.save_as && (
        <div className="border-t border-border/40 px-2.5 py-1 text-2xs text-muted-foreground">
          → <code className="font-mono">{s.save_as}</code>
        </div>
      )}
      <Handle type="source" position={Position.Right} style={{ background: data.color, width: 8, height: 8 }} />
    </div>
  );
}

/** Right-rail form for editing a single step's fields. Type switching
 *  preserves id + save_as but drops type-specific fields that no longer
 *  apply, since mixed-up shapes confuse the executor. */
function StepEditor({ step, allIds, onChange, onDelete, onClose }: {
  step: WorkflowStep;
  allIds: string[];
  onChange: (patch: Partial<WorkflowStep>) => void;
  onDelete: () => void;
  onClose: () => void;
}) {
  const m = typeMeta(step.type);
  const Icon = m.icon;

  return (
    <aside className="flex w-80 shrink-0 flex-col gap-3 overflow-y-auto rounded-xl border border-border bg-card/40 p-3">
      <header className="flex items-center gap-2">
        <Icon className="h-4 w-4" style={{ color: m.color }} />
        <span className="text-sm font-medium">{m.label} step</span>
        <button onClick={onClose} className="ml-auto text-2xs text-muted-foreground hover:text-foreground">Close</button>
      </header>

      <Field label="ID">
        <input
          value={step.id}
          onChange={(e) => {
            const v = e.target.value.replace(/\s+/g, '_');
            if (v && !allIds.includes(v)) onChange({ id: v });
          }}
          className="qr-input"
        />
      </Field>

      <Field label="Type">
        <select
          value={step.type}
          onChange={(e) => onChange({ type: e.target.value as WorkflowStepType })}
          className="qr-input"
        >
          {NODE_TYPES_ALL.map((t) => <option key={t} value={t}>{typeMeta(t).label}</option>)}
        </select>
      </Field>

      {step.type === 'prompt' && (
        <Field label="Prompt">
          <textarea
            value={step.prompt ?? ''}
            onChange={(e) => onChange({ prompt: e.target.value })}
            rows={5}
            className="qr-input min-h-[7rem] resize-y py-2"
          />
        </Field>
      )}

      {step.type === 'tool' && (
        <>
          <Field label="Tool name">
            <input value={step.tool ?? ''} onChange={(e) => onChange({ tool: e.target.value })} className="qr-input" />
          </Field>
          <Field label="Args (JSON)">
            <JsonField value={step.args} onChange={(args) => onChange({ args: args as Record<string, unknown> })} />
          </Field>
        </>
      )}

      {step.type === 'api' && (
        <>
          <Field label="Method">
            <select value={step.method ?? 'GET'} onChange={(e) => onChange({ method: e.target.value })} className="qr-input">
              {['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map((m) => <option key={m} value={m}>{m}</option>)}
            </select>
          </Field>
          <Field label="URL">
            <input value={step.url ?? ''} onChange={(e) => onChange({ url: e.target.value })} className="qr-input" />
          </Field>
          <Field label="Body (JSON)">
            <JsonField value={step.body} onChange={(body) => onChange({ body: body as Record<string, unknown> })} />
          </Field>
        </>
      )}

      {step.type === 'delegate' && (
        <>
          <Field label="Soul key">
            <input value={step.soul_key ?? ''} onChange={(e) => onChange({ soul_key: e.target.value })} className="qr-input" />
          </Field>
          <Field label="Task">
            <textarea
              value={step.task ?? ''}
              onChange={(e) => onChange({ task: e.target.value })}
              rows={3}
              className="qr-input min-h-[5rem] resize-y py-2"
            />
          </Field>
        </>
      )}

      {step.type === 'collect' && (
        <Field label="Fields (comma-separated)">
          <input
            value={(step.fields ?? []).join(', ')}
            onChange={(e) => onChange({ fields: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })}
            className="qr-input"
          />
        </Field>
      )}

      {step.type === 'condition' && (
        <Field label="Branches (value → step id)">
          <JsonField value={step.branches} onChange={(v) => onChange({ branches: (v as Record<string, string>) ?? {} })} />
        </Field>
      )}

      {step.type === 'wait' && (
        <Field label="Args (JSON — e.g. {&quot;seconds&quot;: 60})">
          <JsonField value={step.args} onChange={(args) => onChange({ args: args as Record<string, unknown> })} />
        </Field>
      )}

      {step.type === 'notify' && (
        <Field label="Args (JSON)">
          <JsonField value={step.args} onChange={(args) => onChange({ args: args as Record<string, unknown> })} />
        </Field>
      )}

      <Field label={<span>Save result as <span className="text-muted-foreground">(optional)</span></span>}>
        <input
          value={step.save_as ?? ''}
          onChange={(e) => onChange({ save_as: e.target.value || undefined })}
          placeholder="varname"
          className="qr-input"
        />
      </Field>

      <button
        onClick={onDelete}
        className="mt-2 inline-flex items-center justify-center gap-1.5 rounded-md border border-destructive/30 px-2 py-1.5 text-xs text-destructive hover:bg-destructive/10"
      >
        <Trash2 className="h-3.5 w-3.5" /> Delete step
      </button>
    </aside>
  );
}

function Field({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-2xs font-medium text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

/** Edit an arbitrary JSON-valued field in a textarea, only commit when
 *  parse succeeds. Empty input clears the field. */
function JsonField({ value, onChange }: { value: unknown; onChange: (v: unknown) => void }) {
  const [draft, setDraft] = useState(() => value == null || (typeof value === 'object' && !Object.keys(value).length) ? '' : JSON.stringify(value, null, 2));
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    setDraft(value == null || (typeof value === 'object' && value && !Object.keys(value).length) ? '' : JSON.stringify(value, null, 2));
  }, [value]);
  return (
    <div>
      <textarea
        value={draft}
        rows={4}
        onChange={(e) => {
          setDraft(e.target.value);
          const t = e.target.value.trim();
          if (t === '') { onChange(undefined); setErr(null); return; }
          try {
            onChange(JSON.parse(t));
            setErr(null);
          } catch (ex) {
            setErr(ex instanceof Error ? ex.message : 'Invalid JSON');
          }
        }}
        className="qr-input min-h-[5rem] resize-y py-2 font-mono text-2xs"
      />
      {err && <p className="mt-1 text-2xs text-destructive">{err}</p>}
    </div>
  );
}

/** Rebuild WorkflowStep[] from live graph state. Edges become `next`
 *  pointers (or branch entries if the source is a condition step); the
 *  rest of the step shape lives on node.data.step and survives moves. */
function reconstruct(nodes: FlowNode[], edges: FlowEdge[]): WorkflowStep[] {
  return nodes.map((n) => {
    const s = { ...(n.data as StepNodeData).step };
    const outgoing = edges.filter((e) => e.source === n.id);

    if (s.type === 'condition') {
      const br: Record<string, string> = {};
      // Keep pre-existing branch labels when the user had them in the
      // right-rail JSON editor; fill blanks with positional defaults
      // (branch_0, branch_1…) so the save doesn't silently drop edges.
      const existing = s.branches ?? {};
      const existingReverse = new Map(Object.entries(existing).map(([k, v]) => [v, k]));
      outgoing.forEach((e, i) => {
        const key = existingReverse.get(e.target) ?? `branch_${i}`;
        br[key] = e.target;
      });
      s.branches = br;
      s.next = undefined;
    } else {
      s.branches = undefined;
      s.next = outgoing[0]?.target;
    }
    // Drop keys our payload shouldn't ship with empty/undefined values.
    return pruneUndefined(s);
  });
}

function pruneUndefined(s: WorkflowStep): WorkflowStep {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(s)) {
    if (v === undefined) continue;
    out[k] = v;
  }
  return out as unknown as WorkflowStep;
}

/** Stable shape key for dirty-detection. */
function serializeKey(nodes: FlowNode[], edges: FlowEdge[]): string {
  const n = nodes.map((x) => ({ id: x.id, step: (x.data as StepNodeData).step }));
  const e = edges.map((x) => ({ s: x.source, t: x.target })).sort((a, b) => (a.s + a.t).localeCompare(b.s + b.t));
  return JSON.stringify({ n, e });
}

/** Pick a human-friendly id for a new step — prefix with type + a
 *  zero-padded number high enough not to collide with what's there. */
function genStepId(type: WorkflowStepType, nodes: FlowNode[]): string {
  const used = new Set(nodes.map((n) => n.id));
  let i = 1;
  while (used.has(`${type}_${i}`)) i++;
  return `${type}_${i}`;
}

/** Build @xyflow/react nodes + edges from the WorkflowStep array.
 *  Layout: layered walk from the detected roots following `next`
 *  + `branches`. Good enough for typical workflows; swap in elkjs
 *  if graphs outgrow this heuristic. */
function buildGraph(steps: WorkflowStep[]): { nodes: FlowNode[]; edges: FlowEdge[] } {
  if (steps.length === 0) return { nodes: [], edges: [] };

  const byId = new Map(steps.map((s) => [s.id, s]));
  const positions = computeLayout(steps, byId);

  const nodes: FlowNode[] = steps.map((s) => {
    const meta = typeMeta(s.type);
    const pos = positions.get(s.id) ?? { x: 0, y: 0 };
    return {
      id: s.id,
      type: 'step',
      position: pos,
      data: { step: s, color: meta.color, label: meta.label, icon: meta.icon } as StepNodeData,
    };
  });

  const edges: FlowEdge[] = [];
  for (const s of steps) {
    // Primary linear edge via `next`
    if (s.next && byId.has(s.next)) {
      edges.push({
        id: `${s.id}->${s.next}`,
        source: s.id,
        target: s.next,
        type: 'smoothstep',
        animated: false,
        style: { stroke: '#71717a', strokeWidth: 1.5 },
      });
    }
    // Branch edges for condition steps
    if (s.branches) {
      for (const [value, targetId] of Object.entries(s.branches)) {
        if (!byId.has(targetId)) continue;
        edges.push({
          id: `${s.id}-${value}->${targetId}`,
          source: s.id,
          target: targetId,
          type: 'smoothstep',
          label: value,
          labelStyle: { fontSize: 10, fontFamily: 'ui-monospace', fill: '#a1a1aa' }, // ok — ReactFlow SVG label, not a page font
          labelBgStyle: { fill: '#18181b', fillOpacity: 0.8 },
          labelBgPadding: [4, 2],
          style: { stroke: '#f59e0b', strokeWidth: 1.5, strokeDasharray: '4 2' },
        });
      }
    }
  }

  return { nodes, edges };
}

/** Simple layered layout: BFS from roots, column per depth. Siblings
 *  stack vertically. Orphan steps land in the deepest column. */
function computeLayout(
  steps: WorkflowStep[],
  byId: Map<string, WorkflowStep>,
): Map<string, { x: number; y: number }> {
  const COL_W = 280;
  const ROW_H = 120;
  const positions = new Map<string, { x: number; y: number }>();
  const depth = new Map<string, number>();
  const visited = new Set<string>();

  // Find roots — any step not referenced as a `next` or branch target.
  const targets = new Set<string>();
  for (const s of steps) {
    if (s.next) targets.add(s.next);
    for (const t of Object.values(s.branches ?? {})) targets.add(t);
  }
  const roots = steps.filter((s) => !targets.has(s.id));
  if (roots.length === 0 && steps.length > 0) roots.push(steps[0]!);

  const queue: string[] = roots.map((s) => { depth.set(s.id, 0); return s.id; });
  while (queue.length) {
    const cur = queue.shift()!;
    if (visited.has(cur)) continue;
    visited.add(cur);
    const s = byId.get(cur);
    if (!s) continue;
    const d = depth.get(cur) ?? 0;
    const children: string[] = [];
    if (s.next && byId.has(s.next)) children.push(s.next);
    for (const t of Object.values(s.branches ?? {})) if (byId.has(t)) children.push(t);
    for (const c of children) {
      if (!depth.has(c)) depth.set(c, d + 1);
      queue.push(c);
    }
  }

  // Orphans — disconnected from any root; stick them in the last column.
  const maxDepth = depth.size > 0 ? Math.max(0, ...Array.from(depth.values())) : 0;
  for (const s of steps) {
    if (!depth.has(s.id)) depth.set(s.id, maxDepth + 1);
  }

  // Group by depth, stack vertically.
  const cols = new Map<number, string[]>();
  for (const s of steps) {
    const d = depth.get(s.id) ?? 0;
    if (!cols.has(d)) cols.set(d, []);
    cols.get(d)!.push(s.id);
  }
  for (const [d, ids] of cols) {
    ids.forEach((sid, i) => {
      positions.set(sid, { x: d * COL_W, y: i * ROW_H });
    });
  }
  return positions;
}

// ─── Settings view ────────────────────────────────────────────────
//
// Edit name, description, trigger_type/trigger_config, enabled toggle.
// Calls PUT /workflows/{id} with only the changed fields.

const TRIGGER_TYPES = ['manual', 'webhook', 'cron', 'channel_message', 'event'] as const;

function SettingsView({ wf, onSaved }: { wf: Workflow; onSaved: () => void }) {
  const [name, setName] = useState(wf.name ?? '');
  const [description, setDescription] = useState(wf.description ?? '');
  const [triggerType, setTriggerType] = useState(wf.trigger_type ?? 'manual');
  const [triggerConfig, setTriggerConfig] = useState(
    () => wf.trigger_config && Object.keys(wf.trigger_config as object).length
      ? JSON.stringify(wf.trigger_config, null, 2)
      : ''
  );
  const [triggerConfigErr, setTriggerConfigErr] = useState<string | null>(null);
  const [variables, setVariables] = useState(
    () => wf.variables && Object.keys(wf.variables as object).length
      ? JSON.stringify(wf.variables, null, 2)
      : ''
  );
  const [variablesErr, setVariablesErr] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const parseJson = (s: string, setErr: (e: string | null) => void): unknown => {
    const t = s.trim();
    if (!t) return undefined;
    try { const v = JSON.parse(t); setErr(null); return v; }
    catch (e) { setErr(e instanceof Error ? e.message : 'Invalid JSON'); return null; }
  };

  const handleSave = async () => {
    const tc = parseJson(triggerConfig, setTriggerConfigErr);
    const vars = parseJson(variables, setVariablesErr);
    if (tc === null || vars === null) return; // JSON parse failed — errors shown inline
    setSaving(true);
    setSaveError(null);
    try {
      await workflows.update(wf.id, {
        name: name.trim() || wf.name,
        description: description.trim() || undefined,
        trigger_type: triggerType,
        ...(tc !== undefined && { trigger_config: tc }),
        ...(vars !== undefined && { variables: vars }),
      });
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
      onSaved();
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-xl space-y-5 py-4">
        <SettingField label="Name">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="qr-input"
          />
        </SettingField>

        <SettingField label="Description">
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            className="qr-input min-h-[4rem] resize-y py-2"
          />
        </SettingField>

        <SettingField label="Trigger type">
          <select
            value={triggerType}
            onChange={(e) => setTriggerType(e.target.value)}
            className="qr-input"
          >
            {TRIGGER_TYPES.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
          <p className="mt-1 text-2xs text-muted-foreground">
            {triggerType === 'manual' && 'Run via API or the Run button only.'}
            {triggerType === 'webhook' && 'Fire on POST /webhooks/{workflow_id}. Set secret in trigger config.'}
            {triggerType === 'cron' && 'Schedule via trigger config: {"cron": "0 9 * * 1-5"}.'}
            {triggerType === 'channel_message' && 'Fire when a channel receives a message matching trigger config filter.'}
            {triggerType === 'event' && 'Subscribe to a platform event in trigger config.'}
          </p>
        </SettingField>

        {triggerType !== 'manual' && (
          <SettingField label="Trigger config (JSON)">
            <textarea
              value={triggerConfig}
              rows={4}
              onChange={(e) => {
                setTriggerConfig(e.target.value);
                const t = e.target.value.trim();
                if (t === '') { setTriggerConfigErr(null); return; }
                try { JSON.parse(t); setTriggerConfigErr(null); }
                catch (ex) { setTriggerConfigErr(ex instanceof Error ? ex.message : 'Invalid JSON'); }
              }}
              className="qr-input min-h-[5rem] resize-y py-2 font-mono text-2xs"
              placeholder="{}"
            />
            {triggerConfigErr && <p className="mt-1 text-2xs text-destructive">{triggerConfigErr}</p>}
          </SettingField>
        )}

        <SettingField label="Workflow variables (JSON)">
          <textarea
            value={variables}
            rows={4}
            onChange={(e) => {
              setVariables(e.target.value);
              const t = e.target.value.trim();
              if (t === '') { setVariablesErr(null); return; }
              try { JSON.parse(t); setVariablesErr(null); }
              catch (ex) { setVariablesErr(ex instanceof Error ? ex.message : 'Invalid JSON'); }
            }}
            className="qr-input min-h-[5rem] resize-y py-2 font-mono text-2xs"
            placeholder='{"key": "default_value"}'
          />
          {variablesErr && <p className="mt-1 text-2xs text-destructive">{variablesErr}</p>}
          <p className="mt-1 text-2xs text-muted-foreground">
            Default values available to all steps as <code className="font-mono">{'{{key}}'}</code>.
          </p>
        </SettingField>

        {saveError && (
          <p className="text-xs text-destructive">{saveError}</p>
        )}

        <button
          onClick={handleSave}
          disabled={saving || !!triggerConfigErr || !!variablesErr}
          className={cn(
            'inline-flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50',
            saved
              ? 'bg-emerald-500/15 text-emerald-500'
              : 'bg-primary text-primary-foreground hover:bg-primary/90',
          )}
        >
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : saved ? <CheckCircle2 className="h-3.5 w-3.5" /> : <Save className="h-3.5 w-3.5" />}
          {saved ? 'Saved' : 'Save settings'}
        </button>
      </div>
    </div>
  );
}

function SettingField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-xs font-medium text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

// ─── History view ─────────────────────────────────────────────────

function HistoryView({ runs }: { runs: WorkflowRun[] }) {
  if (runs.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center rounded-xl border border-dashed border-border/60 bg-card/40">
        <p className="text-sm text-muted-foreground">
          No runs yet. Click <strong>Run</strong> above to kick one off.
        </p>
      </div>
    );
  }
  return (
    <div className="flex-1 overflow-auto rounded-xl border border-border bg-card/40">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-border/60 bg-muted/30 text-2xs uppercase tracking-wider text-muted-foreground">
            <th className="px-4 py-2 text-left font-medium">Status</th>
            <th className="px-4 py-2 text-left font-medium">Run ID</th>
            <th className="px-4 py-2 text-left font-medium">Started</th>
            <th className="px-4 py-2 text-right font-medium">Duration</th>
            <th className="px-4 py-2 text-left font-medium">Result / error</th>
          </tr>
        </thead>
        <tbody>
          {runs.map((r) => <RunRow key={r.id} run={r} />)}
        </tbody>
      </table>
    </div>
  );
}

function RunRow({ run }: { run: WorkflowRun }) {
  const dur = run.completed_at
    ? Date.parse(run.completed_at) - Date.parse(run.started_at)
    : null;
  const statusIcon =
    run.status === 'completed' ? <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500" />
    : run.status === 'failed'    ? <XCircle className="h-3.5 w-3.5 text-destructive" />
    : run.status === 'running'   ? <Loader2 className="h-3.5 w-3.5 animate-spin text-primary" />
    : <Clock className="h-3.5 w-3.5 text-muted-foreground" />;

  return (
    <tr className={cn('border-b border-border/30 last:border-0', run.status === 'failed' && 'bg-destructive/5')}>
      <td className="px-4 py-2">
        <span className="inline-flex items-center gap-1.5">
          {statusIcon}
          <span className="font-mono">{run.status}</span>
        </span>
      </td>
      <td className="px-4 py-2 font-mono text-muted-foreground">{run.id.slice(0, 8)}</td>
      <td className="px-4 py-2 text-muted-foreground">{new Date(run.started_at).toLocaleString()}</td>
      <td className="px-4 py-2 text-right font-mono text-muted-foreground">
        {dur != null ? `${(dur / 1000).toFixed(1)}s` : '—'}
      </td>
      <td className="max-w-md truncate px-4 py-2 text-muted-foreground" title={run.error ?? run.result ?? ''}>
        {run.error ? <span className="text-destructive">{run.error}</span> : run.result ?? ''}
      </td>
    </tr>
  );
}

// ─── Source view ──────────────────────────────────────────────────
//
// Read-only. The backend workflow PUT endpoint exists but there's no
// api.ts wrapper yet, and shipping a broken save button is worse
// than not shipping one. Users who need to edit steps today do so
// via the admin API or by replacing the workflow.

function SourceView({ wf }: { wf: Workflow }) {
  const steps = parseWorkflowSteps(wf.steps);
  const pretty = JSON.stringify(
    {
      id: wf.id,
      name: wf.name,
      description: wf.description,
      enabled: wf.enabled,
      trigger_type: wf.trigger_type,
      steps,
    },
    null,
    2,
  );
  return (
    <div className="flex-1 overflow-auto rounded-xl border border-border bg-card/40">
      <pre className="whitespace-pre-wrap break-all p-4 font-mono text-xs leading-relaxed text-foreground/80">
        {pretty}
      </pre>
    </div>
  );
}
