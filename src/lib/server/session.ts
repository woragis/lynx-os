import "server-only";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";

export type LynxUser = {
  id: string;
  email: string;
  displayName: string;
  role: "owner" | "admin" | "operator" | "viewer";
};

const apiOrigin = () => process.env.LYNX_GO_INTERNAL_URL?.replace(/\/$/, "");

export function isAuthConfigured() {
  return Boolean(apiOrigin() && process.env.LYNX_GO_API_URL);
}

export async function currentUser(): Promise<{ user: LynxUser; sessionId: string } | null> {
  const origin = apiOrigin();
  if (!origin) return null;
  try {
    const cookieHeader = (await cookies()).getAll().map(({ name, value }) => `${name}=${value}`).join("; ");
    const response = await fetch(`${origin}/api/v1/auth/me`, {
      headers: { cookie: cookieHeader },
      cache: "no-store",
      signal: AbortSignal.timeout(2500),
    });
    if (!response.ok) return null;
    const result = await response.json() as { data?: { user?: LynxUser; sessionId?: string } };
    if (!result.data?.user || !result.data.sessionId) return null;
    return { user: result.data.user, sessionId: result.data.sessionId };
  } catch {
    return null;
  }
}

export async function hasSession() {
  return Boolean(await currentUser());
}

export async function requireSession() {
  const session = await currentUser();
  if (!session) redirect("/login");
  return session.user;
}

export async function requireRoles(roles: LynxUser["role"][]) {
  const user = await requireSession();
  if (!roles.includes(user.role)) redirect("/accounts?erro=permissao");
  return user;
}
