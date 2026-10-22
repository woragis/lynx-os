"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { ensureCsrfToken } from "@/lib/auth-client";

export function PasswordRecoveryForm() {
  const [sent, setSent] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy(true);
    setError("");
    try {
      const csrf = await ensureCsrfToken();
      const response = await fetch("/api/v1/auth/password/forgot", {
        method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ email: form.get("email") }),
      });
      if (response.status !== 202) throw new Error("request failed");
      setSent(true);
    } catch {
      setError("Não foi possível processar agora. Tente novamente mais tarde.");
    } finally {
      setBusy(false);
    }
  }

  if (sent) return <div className="recovery-result"><p>Se o endereço estiver associado a uma conta, enviaremos instruções para redefinir a senha.</p><Link href="/login">Voltar ao login</Link></div>;
  return <>
    {error && <p className="error-banner" role="alert">{error}</p>}
    <form onSubmit={submit} className="stack-form">
      <label>E-mail<input name="email" type="email" autoComplete="email" required /></label>
      <button className="button button-dark" type="submit" disabled={busy}>{busy ? "Enviando…" : "Enviar instruções"}</button>
    </form>
    <Link className="auth-back-link" href="/login">Voltar ao login</Link>
  </>;
}
