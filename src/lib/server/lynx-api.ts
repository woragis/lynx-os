import "server-only";

import { cookies } from "next/headers";

export class LynxApiError extends Error {
  constructor(message: string, readonly status: number, readonly code?: string) { super(message); }
}

export async function lynxApi<T>(path: string, init: RequestInit = {}): Promise<T> {
  const origin = process.env.LYNX_GO_INTERNAL_URL?.replace(/\/$/, "");
  if (!origin) throw new Error("LYNX_GO_INTERNAL_URL não está configurada.");
  const store = await cookies();
  const cookieHeader = store.getAll().map(({ name, value }) => `${name}=${value}`).join("; ");
  const csrf = store.get("lynx_csrf")?.value;
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body) headers.set("Content-Type", "application/json");
  if (cookieHeader) headers.set("Cookie", cookieHeader);
  if (csrf && init.method && !["GET", "HEAD", "OPTIONS"].includes(init.method.toUpperCase())) headers.set("X-CSRF-Token", csrf);
  let response: Response;
  try { response = await fetch(`${origin}/api/v1${path.startsWith("/") ? path : `/${path}`}`, { ...init, headers, cache: "no-store", signal: init.signal ?? AbortSignal.timeout(35000) }); }
  catch { throw new Error("Não foi possível conectar à API Lynx OS."); }
  const body: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    const error = body && typeof body === "object" && "error" in body ? (body as { error?: { message?: string; code?: string } }).error : undefined;
    throw new LynxApiError(error?.message ?? `A API Lynx OS respondeu HTTP ${response.status}.`, response.status, error?.code);
  }
  return body as T;
}
