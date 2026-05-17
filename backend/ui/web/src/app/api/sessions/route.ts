// @ts-nocheck
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import { generateText } from "ai";

const GO_API = "http://localhost:4200/v1";
const AUTH = { Authorization: "Bearer test123", "Content-Type": "application/json" };

const provider = createOpenAICompatible({
  baseURL: GO_API, name: "qorven",
  headers: { Authorization: "Bearer test123" },
});

export async function GET(req: Request) {
  const url = new URL(req.url);
  const agentId = url.searchParams.get("agentId");
  const res = await fetch(`${GO_API}/sessions`, { headers: AUTH });
  const data = await res.json();
  let sessions = data.sessions || [];
  if (agentId) sessions = sessions.filter((s: any) => s.agent_id === agentId && s.channel !== "cron");
  return Response.json({ sessions });
}

export async function POST(req: Request) {
  const body = await req.json();

  if (body.action === "create") {
    const res = await fetch(`${GO_API}/sessions`, {
      method: "POST", headers: AUTH,
      body: JSON.stringify({ agent_id: body.agentId || "default", channel: "web", label: body.label || "" }),
    });
    const session = await res.json();
    return Response.json(session);
  }

  if (body.action === "auto-name") {
    try {
      const result = await generateText({
        model: provider("glm-4.7-flash"),
        prompt: `Generate a very short title (3-5 words) for a chat starting with: "${body.firstMessage}". Return ONLY the title.`,
      });
      return Response.json({ name: result.text.trim().replace(/^["']|["']$/g, "").slice(0, 50) });
    } catch {
      return Response.json({ name: body.firstMessage?.slice(0, 40) || "Chat" });
    }
  }

  if (body.action === "get") {
    const res = await fetch(`${GO_API}/sessions/${body.id}`, { headers: AUTH });
    if (!res.ok) return Response.json(null);
    return Response.json(await res.json());
  }

  if (body.action === "save-messages") {
    // Messages are saved by the agent loop in Go backend
    // This is a no-op now — the Go backend handles persistence
    return Response.json({ ok: true });
  }

  if (body.action === "delete") {
    await fetch(`${GO_API}/sessions/${body.id}`, { method: "DELETE", headers: AUTH });
    return Response.json({ ok: true });
  }

  return Response.json({ error: "unknown action" }, { status: 400 });
}
