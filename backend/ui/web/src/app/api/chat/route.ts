import { smoothStream, streamText } from "ai";
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";

export const maxDuration = 120;

const GO_BACKEND = "http://localhost:4200/v1/chat/completions";
const AUTH = "Bearer test123";

const provider = createOpenAICompatible({
  baseURL: "http://localhost:4200/v1",
  name: "qorven",
  headers: { Authorization: AUTH },
});

export async function POST(req: Request) {
  const { messages, model = "kimi-k2.5", agent_id, session_id } = await req.json();

  const plainMessages = messages.map((m: any) => {
    const content = m.parts
      ? m.parts.filter((p: any) => p.type === "text").map((p: any) => p.text).join("\n")
      : m.content || "";
    return { role: m.role as "user" | "assistant" | "system", content };
  });

  // Always use agent loop (tools, memory, skills)
  // Auto-resolve agent AND session on the server side
  let resolvedAgentId = agent_id || "";
  if (!resolvedAgentId) {
    try {
      const agentsResp = await fetch("http://localhost:4200/v1/agents", { headers: { Authorization: AUTH } });
      const agentsData = await agentsResp.json();
      if (agentsData.agents?.length > 0) resolvedAgentId = agentsData.agents[0].id;
    } catch {}
  }

  // Auto-resolve session: find most recent for this agent, or create one
  let resolvedSessionId = session_id || "";
  if (!resolvedSessionId && resolvedAgentId) {
    try {
      const sessResp = await fetch(`http://localhost:4200/v1/sessions?agent_id=${resolvedAgentId}`, { headers: { Authorization: AUTH } });
      const sessData = await sessResp.json();
      const activeSessions = sessData.sessions || [];
      if (activeSessions.length > 0) {
        resolvedSessionId = activeSessions[0].id;
      } else {
        // Create new session
        const createResp = await fetch("http://localhost:4200/v1/sessions", {
          method: "POST", headers: { "Content-Type": "application/json", Authorization: AUTH },
          body: JSON.stringify({ agent_id: resolvedAgentId, channel: "web" }),
        });
        const newSess = await createResp.json();
        resolvedSessionId = newSess.id;
      }
    } catch {}
  }

  if (resolvedAgentId) {
    const goResp = await fetch(GO_BACKEND, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: AUTH },
      body: JSON.stringify({
        model,
        messages: plainMessages,
        stream: true,
        agent_id: resolvedAgentId,
        session_id: resolvedSessionId,
      }),
    });

    if (!goResp.ok || !goResp.body) {
      return new Response(JSON.stringify({ error: "Backend error" }), { status: 502 });
    }

    // Pass through SSE from Go backend, convert to AI SDK format
    const encoder = new TextEncoder();
    const decoder = new TextDecoder();
    const readable = new ReadableStream({
      async start(controller) {
        const reader = goResp.body!.getReader();
        let buffer = "";
        let started = false;
        let currentToolId = "";

        controller.enqueue(encoder.encode("data: {\"type\":\"start\"}\n\n"));
        controller.enqueue(encoder.encode("data: {\"type\":\"start-step\"}\n\n"));

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() || "";

          for (const line of lines) {
            if (!line.startsWith("data: ")) continue;
            const data = line.slice(6).trim();
            if (data === "[DONE]") {
              if (started) controller.enqueue(encoder.encode(`data: {"type":"text-end","id":"txt-0"}\n\n`));
              controller.enqueue(encoder.encode(`data: {"type":"finish-step"}\n\n`));
              controller.enqueue(encoder.encode(`data: {"type":"finish","finishReason":"stop"}\n\n`));
              controller.enqueue(encoder.encode("data: [DONE]\n\n"));
              controller.close();
              return;
            }
            try {
              const parsed = JSON.parse(data);
              // Tool events → emit as tool-call annotations for rich rendering
              if (parsed.type === "tool_start") {
                if (started) {
                  controller.enqueue(encoder.encode(`data: {"type":"text-end","id":"txt-0"}\n\n`));
                  started = false;
                }
                currentToolId = parsed.name || "tool";
                // Emit as visible text with tool marker
                const toolMarker = `\n\n---tool:${currentToolId}:running---\n\n`;
                controller.enqueue(encoder.encode(`data: {"type":"text-start","id":"tool-s-${Date.now()}"}\n\n`));
                controller.enqueue(encoder.encode(`data: {"type":"text-delta","id":"tool-s-${Date.now()}","delta":${JSON.stringify(toolMarker)}}\n\n`));
                controller.enqueue(encoder.encode(`data: {"type":"text-end","id":"tool-s-${Date.now()}"}\n\n`));
                continue;
              }
              if (parsed.type === "tool_result") {
                // Don't emit raw result — LLM will respond with formatted version
                // Just mark the tool as complete
                const toolMarker = `\n---tool:${currentToolId}:complete---\n`;
                controller.enqueue(encoder.encode(`data: {"type":"text-start","id":"tool-r-${Date.now()}"}\n\n`));
                controller.enqueue(encoder.encode(`data: {"type":"text-delta","id":"tool-r-${Date.now()}","delta":${JSON.stringify(toolMarker)}}\n\n`));
                controller.enqueue(encoder.encode(`data: {"type":"text-end","id":"tool-r-${Date.now()}"}\n\n`));
                continue;
              }
              // Text delta
              const delta = parsed.choices?.[0]?.delta;
              if (delta?.content) {
                if (!started) {
                  controller.enqueue(encoder.encode(`data: {"type":"text-start","id":"txt-0"}\n\n`));
                  started = true;
                }
                controller.enqueue(encoder.encode(`data: {"type":"text-delta","id":"txt-0","delta":${JSON.stringify(delta.content)}}\n\n`));
              }
              if (delta?.reasoning_content) {
                controller.enqueue(encoder.encode(`data: {"type":"reasoning-start","id":"r-0"}\n\n`));
                controller.enqueue(encoder.encode(`data: {"type":"reasoning-delta","id":"r-0","delta":${JSON.stringify(delta.reasoning_content)}}\n\n`));
                controller.enqueue(encoder.encode(`data: {"type":"reasoning-end","id":"r-0"}\n\n`));
              }
            } catch {}
          }
        }
        controller.close();
      },
    });

    return new Response(readable, {
      headers: { "Content-Type": "text/event-stream", "Cache-Control": "no-cache", Connection: "keep-alive" },
    });
  }

  // No agent_id: direct provider passthrough (backward compat)
  const result = streamText({
    model: provider(model),
    system: "You are Qorven AI, a helpful assistant.",
    messages: plainMessages,
    experimental_transform: smoothStream({ chunking: "word" }),
  });

  return result.toUIMessageStreamResponse();
}
