"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { ensureCsrfToken } from "@/lib/auth-client";

export function LoginForm({ disabled }: { disabled: boolean }) {
  const router = useRouter();
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage("");
    setBusy(true);
    const form = new FormData(event.currentTarget);
    try {
      const csrf = await ensureCsrfToken();
      const response = await fetch("/api/v1/auth/login", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ email: form.get("email"), password: form.get("password") }),
      });
      if (response.ok) {
        router.replace("/accounts");
        router.refresh();
        return;
      }
      if (response.status === 429) setMessage("Muitas tentativas. Aguarde alguns minutos e tente novamente.");
      else if (response.status === 503) setMessage("Autenticação temporariamente indisponível. Tente novamente em instantes.");
      else setMessage("E-mail ou senha inválidos.");
    } catch {
      setMessage("Não foi possível conectar ao serviço de autenticação.");
    } finally {
      setBusy(false);
    }
  }

  return <>
    {message && <p className="error-banner" role="alert">{message}</p>}
    <form onSubmit={submit} className="stack-form">
      <label>E-mail<input name="email" type="email" autoComplete="username" required disabled={disabled || busy} /></label>
      <label>Senha<input name="password" type="password" autoComplete="current-password" required disabled={disabled || busy} /></label>
      <button className="button button-dark" type="submit" disabled={disabled || busy}>{busy ? "Entrando…" : "Entrar"} <span aria-hidden>↗</span></button>
    </form>
  </>;
}
