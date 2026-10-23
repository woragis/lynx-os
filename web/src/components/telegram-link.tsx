"use client";

import { useState } from "react";
import { csrfToken, ensureCsrfToken } from "@/lib/auth-client";

export function TelegramLink() {
  const [code, setCode] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  async function generate() {
    setBusy(true);
    setMessage("");
    setCode("");
    try {
      const csrf = csrfToken() || await ensureCsrfToken();
      const response = await fetch("/api/v1/auth/telegram/link-code", {
        method: "POST", credentials: "same-origin", headers: { "X-CSRF-Token": csrf },
      });
      const result = await response.json() as { data?: { code?: string } };
      if (!response.ok || !result.data?.code) throw new Error("link code unavailable");
      setCode(result.data.code);
    } catch {
      setMessage("Não foi possível gerar um código. Tente novamente.");
    } finally {
      setBusy(false);
    }
  }

  return <section className="panel telegram-link-panel">
    <div><h2>Vincular Telegram</h2><p>Gere um código temporário e envie ao Lynx AI Assistant usando <code>/link código</code>. O código pode ser usado uma vez e expira em 10 minutos.</p></div>
    <button type="button" className="button button-dark" onClick={generate} disabled={busy}>{busy ? "Gerando…" : "Gerar código"}</button>
    {code && <div className="telegram-link-code"><code>{code}</code><button type="button" className="quiet-button" onClick={() => navigator.clipboard.writeText(code)}>Copiar</button></div>}
    {message && <p className="security-message" role="alert">{message}</p>}
  </section>;
}
