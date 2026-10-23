"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { ensureCsrfToken } from "@/lib/auth-client";

export function LogoutButton() {
  const router = useRouter();
  const [busy, setBusy] = useState(false);

  async function logout() {
    setBusy(true);
    try {
      const csrf = await ensureCsrfToken();
      await fetch("/api/v1/auth/logout", {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-CSRF-Token": csrf },
      });
    } catch {
      // Navegar para login mesmo se o servidor já tiver encerrado a sessão.
    } finally {
      router.replace("/login");
      router.refresh();
    }
  }

  return <button className="topbar-logout" type="button" onClick={logout} disabled={busy}>{busy ? "Saindo…" : "Sair ↗"}</button>;
}
