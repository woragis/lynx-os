"use client";

import { useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ensureCsrfToken } from "@/lib/auth-client";

type Mode = "reset" | "invitation";

export function OneTimeAccessForm({ mode, token }: { mode: Mode; token: string }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  useEffect(() => {
    if (token) window.history.replaceState(null, "", window.location.pathname);
  }, [token]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const password = String(form.get("password") ?? "");
    if (password.length < 12 || password.length > 72) {
      setMessage("A senha precisa ter entre 12 e 72 caracteres.");
      return;
    }
    if (password !== form.get("confirmation")) {
      setMessage("As senhas não conferem.");
      return;
    }
    if (!token) {
      setMessage("Este link está incompleto. Solicite um novo.");
      return;
    }
    setBusy(true);
    setMessage("");
    try {
      const csrf = await ensureCsrfToken();
      const path = mode === "reset" ? "/api/v1/auth/password/reset" : "/api/v1/auth/invitations/accept";
      const response = await fetch(path, {
        method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ token, password }),
      });
      if (!response.ok) {
        setMessage(mode === "reset" ? "Link inválido ou expirado. Solicite outra redefinição." : "Convite inválido ou expirado. Peça um novo link ao owner.");
        return;
      }
      router.replace(mode === "reset" ? "/login?sucesso=senha" : "/login?sucesso=convite");
      router.refresh();
    } catch {
      setMessage("Não foi possível conectar ao serviço de autenticação.");
    } finally {
      setBusy(false);
    }
  }

  return <>
    {message && <p className="error-banner" role="alert">{message}</p>}
    <form onSubmit={submit} className="stack-form">
      <label>Nova senha<input name="password" type="password" autoComplete="new-password" minLength={12} maxLength={72} required /></label>
      <label>Confirme a nova senha<input name="confirmation" type="password" autoComplete="new-password" minLength={12} maxLength={72} required /></label>
      <button className="button button-dark" type="submit" disabled={busy}>{busy ? "Salvando…" : "Definir senha"}</button>
    </form>
    <Link className="auth-back-link" href="/login">Voltar ao login</Link>
  </>;
}
