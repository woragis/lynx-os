"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { csrfToken, ensureCsrfToken } from "@/lib/auth-client";

type Session = {
  id: string;
  createdAt: string;
  lastUsedAt: string;
  expiresAt: string;
  revokedAt: string | null;
  ipAddress: string;
  userAgent: string;
  current: boolean;
};

export function SessionManager() {
  const router = useRouter();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    const response = await fetch("/api/v1/auth/sessions", { credentials: "same-origin", cache: "no-store" });
    if (response.status === 401) {
      router.replace("/login");
      return;
    }
    if (!response.ok) throw new Error("Não foi possível carregar as sessões.");
    const result = await response.json() as { data: Session[] };
    setSessions(result.data);
  }, [router]);

  useEffect(() => {
    let mounted = true;
    fetch("/api/v1/auth/sessions", { credentials: "same-origin", cache: "no-store" })
      .then(async (response) => {
        if (response.status === 401) {
          router.replace("/login");
          return [] as Session[];
        }
        if (!response.ok) throw new Error("Could not list sessions");
        const result = await response.json() as { data: Session[] };
        return result.data;
      })
      .then((items) => { if (mounted) setSessions(items); })
      .catch(() => { if (mounted) setMessage("Não foi possível carregar as sessões."); })
      .finally(() => { if (mounted) setLoading(false); });
    return () => { mounted = false; };
  }, [router]);

  async function revoke(id: string, current: boolean) {
    if (!window.confirm(current ? "Revogar esta sessão encerrará seu acesso atual. Continuar?" : "Revogar esta sessão?")) return;
    setBusy(id);
    setMessage("");
    try {
      const csrf = csrfToken() || await ensureCsrfToken();
      const response = await fetch(`/api/v1/auth/sessions/${encodeURIComponent(id)}`, {
        method: "DELETE", credentials: "same-origin", headers: { "X-CSRF-Token": csrf },
      });
      if (!response.ok) throw new Error("revoke failed");
      if (current) {
        router.replace("/login");
        router.refresh();
      } else await load();
    } catch {
      setMessage("Não foi possível revogar a sessão.");
    } finally {
      setBusy("");
    }
  }

  async function revokeAll() {
    if (!window.confirm("Todas as sessões, inclusive esta, serão encerradas. Continuar?")) return;
    setBusy("all");
    try {
      const csrf = csrfToken() || await ensureCsrfToken();
      await fetch("/api/v1/auth/sessions/revoke-all", {
        method: "POST", credentials: "same-origin", headers: { "X-CSRF-Token": csrf },
      });
      router.replace("/login");
      router.refresh();
    } catch {
      setMessage("Não foi possível encerrar as sessões.");
    } finally {
      setBusy("");
    }
  }

  return <section className="panel-card session-panel">
    <div className="panel-heading"><div><h2>Sessões abertas</h2><p>Encerre acessos que você não reconhece ou não usa mais.</p></div>
      <button type="button" className="button button-dark" onClick={revokeAll} disabled={busy !== "" || sessions.length === 0}>Encerrar todas</button>
    </div>
    {message && <p className="error-banner" role="alert">{message}</p>}
    {loading ? <p>Carregando sessões…</p> : sessions.length === 0 ? <p>Nenhuma sessão encontrada.</p> : <div className="session-list">
      {sessions.map((session) => <article className="session-row" key={session.id}>
        <div><strong>{session.current ? "Esta sessão" : "Outro acesso"}</strong>{session.current && <span className="session-current">ATUAL</span>}
          <p>{session.userAgent || "Dispositivo não identificado"}</p>
          <small>IP {session.ipAddress || "indisponível"} · último uso {new Date(session.lastUsedAt).toLocaleString("pt-BR")}</small>
        </div>
        {session.revokedAt ? <span className="session-revoked">Revogada</span> : <button type="button" className="quiet-button" disabled={busy !== ""} onClick={() => revoke(session.id, session.current)}>{busy === session.id ? "Revogando…" : "Revogar"}</button>}
      </article>)}
    </div>}
  </section>;
}
