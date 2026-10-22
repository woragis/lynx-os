"use client";

export function csrfToken() {
  const value = document.cookie.split(";").map((part) => part.trim()).find((part) => part.startsWith("lynx_csrf="));
  return value ? decodeURIComponent(value.slice("lynx_csrf=".length)) : "";
}

export async function ensureCsrfToken() {
  const existing = csrfToken();
  if (existing) return existing;
  const response = await fetch("/api/v1/auth/csrf", { credentials: "same-origin", cache: "no-store" });
  if (!response.ok) throw new Error("Não foi possível inicializar a segurança da sessão.");
  const result = await response.json() as { data?: { csrfToken?: string } };
  return result.data?.csrfToken ?? csrfToken();
}

export async function refreshSession() {
  const token = await ensureCsrfToken();
  const response = await fetch("/api/v1/auth/refresh", {
    method: "POST",
    credentials: "same-origin",
    headers: { "X-CSRF-Token": token },
  });
  return response.ok;
}
