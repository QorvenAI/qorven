// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Bridge between AI SDK useChat and Qorven's Go backend SSE stream.
// The frontend uses DefaultChatTransport pointing to this route.
// This route proxies to the Go backend and translates Qorven's custom
// SSE events into the AI SDK UIMessageChunk protocol.

import { createUIMessageStreamResponse, generateId, type UIMessageChunk } from 'ai';

const BACKEND = process.env.NEXT_PUBLIC_API_URL
  ? `${process.env.NEXT_PUBLIC_API_URL}/v1`
  : 'http://localhost:8080/v1';

function getToken(req: Request): string {
  const auth = req.headers.get('authorization');
  if (auth?.startsWith('Bearer ')) return auth.slice(7);
  return '';
}

export async function POST(req: Request) {
  const body = await req.json();
  const { messages, agentId, sessionId, systemContext, thinkingLevel } = body as {
    messages: Array<{
      role: string;
      content?: string | unknown[];
      parts?: Array<{ type: string; text?: string; url?: string; mediaType?: string; filename?: string }>;
    }>;
    agentId: string;
    sessionId: string;
    systemContext?: string;
    thinkingLevel?: string;
  };

  const token = getToken(req);

  // Extract text from the last user message.
  // AI SDK v6 sends `parts` array; v5 sent `content` string or array — handle both.
  const lastUser = [...messages].reverse().find((m) => m.role === 'user');
  let message = '';
  if (lastUser?.parts) {
    message = lastUser.parts
      .filter((p) => p.type === 'text')
      .map((p) => p.text ?? '')
      .join('');
  } else if (typeof lastUser?.content === 'string') {
    message = lastUser.content;
  } else if (Array.isArray(lastUser?.content)) {
    message = (lastUser.content as Array<{ type: string; text?: string }>)
      .filter((p) => p.type === 'text')
      .map((p) => p.text ?? '')
      .join('');
  }

  // Extract file attachments from the last user message and inject them into
  // the message text as <attached_file> blocks. The Go backend's
  // ExtractFilesFromMessage() parses these out of the message and presents
  // the content to the LLM as structured context.
  const fileParts = lastUser?.parts?.filter((p) => p.type === 'file' && p.url) ?? [];
  if (fileParts.length > 0) {
    const fileBlocks: string[] = [];
    for (const p of fileParts) {
      const filename = p.filename ?? 'attachment';
      const mediaType = p.mediaType ?? 'application/octet-stream';
      const url = p.url!;

      let content = '';
      if (url.startsWith('data:')) {
        // data:<mediaType>;base64,<data>  OR  data:<mediaType>,<data> (plain text)
        const commaIdx = url.indexOf(',');
        if (commaIdx !== -1) {
          const header = url.slice(5, commaIdx); // e.g. "text/csv;base64"
          const raw = url.slice(commaIdx + 1);
          if (header.endsWith(';base64')) {
            // Decode base64 → UTF-8 text
            try {
              content = Buffer.from(raw, 'base64').toString('utf-8');
            } catch {
              content = raw; // fallback: pass raw
            }
          } else {
            content = decodeURIComponent(raw);
          }
        }
      } else {
        // Remote URL — pass as reference; agent can use web_fetch
        content = `[File available at: ${url}]`;
      }

      // Cap at 80K chars to stay within context limits
      if (content.length > 80000) {
        content = content.slice(0, 80000) + '\n\n[... truncated at 80K characters ...]';
      }
      fileBlocks.push(`<attached_file name="${filename}" type="${mediaType}">\n${content}\n</attached_file>`);
    }
    if (fileBlocks.length > 0) {
      message = (message ? message + '\n\n' : '') + fileBlocks.join('\n\n');
    }
  }

  // Open the Go backend SSE stream
  const upstream = await fetch(`${BACKEND}/chat/completions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify({
      session_id: sessionId,
      agent_id: agentId,
      message,
      stream: true,
      ...(systemContext ? { system_context: systemContext } : {}),
      ...(thinkingLevel && thinkingLevel !== 'off' ? { thinking_level: thinkingLevel } : {}),
    }),
  });

  if (!upstream.ok || !upstream.body) {
    const errText = await upstream.text().catch(() => 'upstream error');
    return new Response(errText, { status: upstream.status });
  }

  // Build a ReadableStream<UIMessageChunk> by translating Qorven SSE events
  const upstreamBody = upstream.body;
  const stream = new ReadableStream<UIMessageChunk>({
    async start(controller) {
      const reader = upstreamBody.getReader();
      const decoder = new TextDecoder();
      let buf = '';
      let textId = generateId();
      let reasoningId = generateId();
      let textStarted = false;
      let reasoningStarted = false;
      const pendingTools = new Map<string, string>(); // uniqueId → toolName
      // Map backend's raw toolCallId → our generated unique id (refreshed each iteration)
      const rawIdMap = new Map<string, string>(); // rawBackendId → currentUniqueId

      const enq = (chunk: UIMessageChunk) => {
        try { controller.enqueue(chunk); } catch { /* stream closed */ }
      };

      try {
        controller.enqueue({ type: 'start' });

        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          buf += decoder.decode(value, { stream: true });

          const lines = buf.split('\n');
          buf = lines.pop() ?? '';

          for (const line of lines) {
            if (!line.startsWith('data: ')) continue;
            const raw = line.slice(6).trim();
            if (raw === '[DONE]') {
              if (textStarted) { enq({ type: 'text-end', id: textId }); textStarted = false; }
              if (reasoningStarted) { enq({ type: 'reasoning-end', id: reasoningId }); reasoningStarted = false; }
              enq({ type: 'finish' });
              controller.close();
              return;
            }

            let evt: Record<string, unknown>;
            try { evt = JSON.parse(raw); } catch { continue; }

            // OpenAI-compat text delta (backend wraps text in choices[0].delta.content)
            const choices = evt.choices as Array<{ delta?: { content?: string; reasoning_content?: string } }> | undefined;
            if (choices?.[0]?.delta?.content) {
              const delta = choices[0].delta.content!;
              if (!textStarted) { enq({ type: 'text-start', id: textId }); textStarted = true; }
              enq({ type: 'text-delta', id: textId, delta });
              continue;
            }
            if (choices?.[0]?.delta?.reasoning_content) {
              const delta = choices[0].delta.reasoning_content!;
              if (!reasoningStarted) { enq({ type: 'reasoning-start', id: reasoningId }); reasoningStarted = true; }
              enq({ type: 'reasoning-delta', id: reasoningId, delta });
              continue;
            }

            const type = evt.type as string | undefined;
            const data = evt.data as Record<string, unknown> | null | undefined;

            if (type === 'stream_reset') {
              // Discard pre-tool narration
              if (textStarted) { enq({ type: 'text-end', id: textId }); textStarted = false; }
              textId = generateId(); // fresh id so next text chunk starts clean
              continue;
            }

            // Backend emits tool info via type="part" events with data.type="tool-call"/"tool-result"
            if (type === 'part') {
              const partType = (data as Record<string, unknown>)?.type as string | undefined;
              if (partType === 'tool-call') {
                const toolName = String((data as Record<string, unknown>)?.toolName ?? 'tool');
                const rawId = String((data as Record<string, unknown>)?.toolCallId ?? generateId());
                const toolArgs = (data as Record<string, unknown>)?.toolArgs ?? {};
                // Generate a fresh unique ID for this iteration — backend reuses the same rawId
                const uniqueId = generateId();
                rawIdMap.set(rawId, uniqueId);
                pendingTools.set(uniqueId, toolName);
                enq({
                  type: 'tool-input-available',
                  toolCallId: uniqueId,
                  toolName,
                  input: toolArgs,
                  dynamic: true,
                });
              } else if (partType === 'tool-result') {
                const toolName = String((data as Record<string, unknown>)?.toolName ?? 'tool');
                const rawId = String((data as Record<string, unknown>)?.toolCallId ?? '');
                const result = (data as Record<string, unknown>)?.toolResult ?? '';
                // Resolve the unique ID we generated for this raw ID
                const uniqueId = rawIdMap.get(rawId) ?? (() => {
                  for (const [id, n] of pendingTools) { if (n === toolName) return id; }
                  return generateId();
                })();
                rawIdMap.delete(rawId);
                pendingTools.delete(uniqueId);
                enq({
                  type: 'tool-output-available',
                  toolCallId: uniqueId,
                  output: result,
                } as unknown as UIMessageChunk);
              } else if (partType === 'sources') {
                const sources = (data as Record<string, unknown>)?.sources;
                if (Array.isArray(sources)) {
                  enq({ type: 'data-sources', id: generateId(), data: sources } as unknown as UIMessageChunk);
                }
              }
              continue;
            }

            // tool_start with data=null is a text-reset signal only (real tool info comes via "part" events)
            // tool_start with data present is a legacy format — emit tool card
            if (type === 'tool_start') {
              if (textStarted) { enq({ type: 'text-end', id: textId }); textStarted = false; }
              textId = generateId();
              if (data) {
                const toolName = String(data?.name ?? data?.tool ?? evt.tool ?? 'tool');
                const toolCallId = String(data?.id ?? evt.tool_id ?? generateId());
                pendingTools.set(toolCallId, toolName);
                enq({
                  type: 'tool-input-available',
                  toolCallId,
                  toolName,
                  input: data?.args ?? data?.arguments ?? {},
                  dynamic: true,
                });
              }
              continue;
            }

            if (type === 'tool_result' || type === 'tool_end') {
              const toolName = String(data?.name ?? data?.tool ?? evt.tool ?? 'tool');
              let toolCallId = String(data?.id ?? evt.tool_id ?? '');
              if (!toolCallId) {
                for (const [id, name] of pendingTools) {
                  if (name === toolName) { toolCallId = id; break; }
                }
              }
              if (!toolCallId) toolCallId = generateId();
              pendingTools.delete(toolCallId);
              enq({
                type: 'tool-output-available',
                toolCallId,
                output: data?.result ?? data?.output ?? '',
              } as unknown as UIMessageChunk);
              continue;
            }

            if (type === 'sources') {
              const sources = Array.isArray(data) ? data : (Array.isArray((data as Record<string, unknown>)?.sources) ? (data as Record<string, unknown>).sources as unknown[] : []);
              // Emit as data chunk so onData callback receives it
              enq({ type: 'data-sources', id: generateId(), data: sources } as unknown as UIMessageChunk);
              continue;
            }

            if (type === 'follow_up') {
              const followUps = Array.isArray(data) ? data : ((data as Record<string, unknown>)?.follow_ups ?? []);
              enq({ type: 'data-follow_ups', id: generateId(), data: followUps } as unknown as UIMessageChunk);
              continue;
            }

            if (type === 'title') {
              const title = typeof data === 'string' ? data : (data as Record<string, unknown>)?.title as string ?? '';
              if (title) enq({ type: 'data-title', id: generateId(), data: title } as unknown as UIMessageChunk);
              continue;
            }

            if (type === 'widget') {
              enq({ type: 'data-widget', id: generateId(), data } as unknown as UIMessageChunk);
              continue;
            }

            if (type === 'error') {
              enq({ type: 'error', errorText: String(data ?? 'Agent error') });
              continue;
            }
          }
        }
      } catch (err) {
        controller.enqueue({ type: 'error', errorText: String(err) });
      } finally {
        if (textStarted) enq({ type: 'text-end', id: textId });
        if (reasoningStarted) enq({ type: 'reasoning-end', id: reasoningId });
        try { enq({ type: 'finish' }); controller.close(); } catch { /* already closed */ }
      }
    },
  });

  return createUIMessageStreamResponse({ stream });
}
