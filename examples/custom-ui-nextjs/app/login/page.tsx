"use client";

// Force dynamic rendering. These pages depend on client-side auth
// state and cannot be statically prerendered — Next 15 would try
// otherwise, and `apiBase()` throws at build time if the env var
// is not baked in. We never want a build-time baked API URL anyway.
export const dynamic = "force-dynamic";

import { useState } from "react";
import { useRouter } from "next/navigation";

import { apiBase } from "@/lib/api";
import { login } from "@/lib/auth";

/**
 * Login page. Posts to /auth/login, stores the token in memory via
 * the auth store, and redirects home.
 *
 * The form works with either username/password or the gateway's
 * static token (if configured). For the token path, the user sets
 * `username=""` and pastes the token as the password — the server
 * rejects an empty username, so this path is only useful when the
 * user has *already* been issued an API key. Document this to agents
 * who ask how to wire a headless login.
 */
export default function LoginPage() {
  const router = useRouter();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      await login(apiBase(), username, password);
      router.replace("/");
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main style={wrap}>
      <h1>Log in to Qorven</h1>
      <p style={{ color: "#475569", marginBottom: "1.5rem" }}>
        Gateway: <code>{apiBase()}</code>
      </p>
      <form onSubmit={onSubmit} style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        <label style={label}>
          Username
          <input
            type="text"
            autoComplete="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoFocus
          />
        </label>
        <label style={label}>
          Password
          <input
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </label>
        <button type="submit" disabled={busy}>
          {busy ? "Signing in…" : "Sign in"}
        </button>
        {err && <p style={errStyle}>{err}</p>}
      </form>
    </main>
  );
}

const wrap: React.CSSProperties = {
  maxWidth: 380,
  margin: "4rem auto",
  padding: "0 1.5rem",
};
const label: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 4,
  fontSize: "0.9rem",
};
const errStyle: React.CSSProperties = {
  color: "#991b1b",
  background: "#fef2f2",
  padding: "0.5rem 0.75rem",
  borderRadius: 4,
  margin: 0,
  fontSize: "0.85rem",
};
