import "server-only";

import { createHmac, timingSafeEqual } from "node:crypto";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

const COOKIE_NAME = "lynx_os_session";
const MAX_AGE_SECONDS = 8 * 60 * 60;

function signature(value: string) {
  const secret = process.env.LYNX_SESSION_SECRET;
  if (!secret || secret.length < 32) throw new Error("LYNX_SESSION_SECRET precisa ter ao menos 32 caracteres.");
  return createHmac("sha256", secret).update(value).digest("base64url");
}

export function isAuthConfigured() {
  return Boolean(process.env.LYNX_ADMIN_EMAIL && process.env.LYNX_ADMIN_PASSWORD && (process.env.LYNX_SESSION_SECRET?.length ?? 0) >= 32);
}

function safeEqual(left: string, right: string) {
  const a = Buffer.from(left);
  const b = Buffer.from(right);
  return a.length === b.length && timingSafeEqual(a, b);
}

export async function signIn(email: string, password: string) {
  const configuredEmail = process.env.LYNX_ADMIN_EMAIL ?? "";
  const configuredPassword = process.env.LYNX_ADMIN_PASSWORD ?? "";
  if (!isAuthConfigured()) throw new Error("Configure LYNX_ADMIN_EMAIL, LYNX_ADMIN_PASSWORD e LYNX_SESSION_SECRET antes do login.");
  if (!safeEqual(email.trim().toLowerCase(), configuredEmail.trim().toLowerCase()) || !safeEqual(password, configuredPassword)) {
    return false;
  }

  const expiresAt = Math.floor(Date.now() / 1000) + MAX_AGE_SECONDS;
  const payload = Buffer.from(JSON.stringify({ email: configuredEmail, expiresAt })).toString("base64url");
  const value = `${payload}.${signature(payload)}`;
  (await cookies()).set(COOKIE_NAME, value, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: MAX_AGE_SECONDS,
  });
  return true;
}

export async function hasSession() {
  if (!isAuthConfigured()) return false;
  const value = (await cookies()).get(COOKIE_NAME)?.value;
  if (!value) return false;
  const [payload, suppliedSignature] = value.split(".");
  if (!payload || !suppliedSignature || !safeEqual(signature(payload), suppliedSignature)) return false;
  try {
    const parsed = JSON.parse(Buffer.from(payload, "base64url").toString("utf8")) as { email?: string; expiresAt?: number };
    return parsed.email === process.env.LYNX_ADMIN_EMAIL && typeof parsed.expiresAt === "number" && parsed.expiresAt > Date.now() / 1000;
  } catch {
    return false;
  }
}

export async function requireSession() {
  if (!(await hasSession())) redirect("/login");
}

export async function signOut() {
  (await cookies()).delete(COOKIE_NAME);
}
