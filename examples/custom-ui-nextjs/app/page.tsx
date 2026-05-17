"use client";

// Force dynamic rendering — see note in app/login/page.tsx.
export const dynamic = "force-dynamic";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { apiBase } from "@/lib/api";
import { bootstrap, getAuth, logout, subscribeAuth, type User } from "@/lib/auth";

/**
 * Home page.
 *
 * On mount: try `bootstrap` to see if we already have a valid
 * session (cookie). If not, redirect to /login.
 *
 * When authenticated, show a one-line "you are alice@tenant" banner
 * and an input for a plan id to open. No plan list because there's
 * no public list-plans endpoint yet — the agent or the real app
 * knows the ids it cares about.
 */
export default function HomePage() {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [planId, setPlanId] = useState("");

  useEffect(() => {
    bootstrap(apiBase())
      .then((u) => {
        if (!u) router.replace("/login");
        else setUser(u);
      })
      .finally(() => setLoading(false));
    return subscribeAuth(() => {
      setUser(getAuth().user);
    });
  }, [router]);

  if (loading) return <main style={wrap}><p>Loading…</p></main>;
  if (!user) return null;

  return (
    <main style={wrap}>
      <header style={header}>
        <div>
          <strong>{user.username}</strong>
          <span style={{ color: "#64748b", marginLeft: 8 }}>
            · tenant {user.tenant_id.slice(0, 8)}…
          </span>
        </div>
        <button onClick={() => logout(apiBase()).then(() => router.replace("/login"))}>
          Log out
        </button>
      </header>

      <section>
        <h2>Open a plan</h2>
        <p style={{ color: "#475569" }}>
          Paste a plan id (UUID) to watch its execution live.
        </p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (planId.trim()) router.push(`/plans/${planId.trim()}`);
          }}
          style={{ display: "flex", gap: 8 }}
        >
          <input
            type="text"
            placeholder="00000000-0000-0000-0000-000000000000"
            value={planId}
            onChange={(e) => setPlanId(e.target.value)}
            style={{ flex: 1, fontFamily: "monospace" }}
          />
          <button type="submit">Open</button>
        </form>
      </section>

      <p style={{ marginTop: "3rem", fontSize: "0.85rem", color: "#64748b" }}>
        Reference UI · see <Link href="/">source</Link> · gateway at <code>{apiBase()}</code>
      </p>
    </main>
  );
}

const wrap: React.CSSProperties = {
  maxWidth: 720,
  margin: "0 auto",
  padding: "2rem 1.5rem",
};
const header: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: "2rem",
  paddingBottom: "1rem",
  borderBottom: "1px solid #e2e8f0",
};
