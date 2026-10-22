"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { refreshSession } from "@/lib/auth-client";

export function SessionKeepalive() {
  const router = useRouter();

  useEffect(() => {
    let refreshing = false;
    let lastRefresh = Date.now();
    const refresh = async () => {
      if (refreshing || Date.now() - lastRefresh < 8 * 60 * 1000) return;
      refreshing = true;
      try {
        if (await refreshSession()) lastRefresh = Date.now();
        else {
          router.replace("/login");
          router.refresh();
        }
      } catch {
        // Uma falha temporária de rede não encerra uma sessão ainda válida.
      } finally {
        refreshing = false;
      }
    };
    const timer = window.setInterval(refresh, 60 * 1000);
    document.addEventListener("visibilitychange", refresh);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", refresh);
    };
  }, [router]);

  return null;
}
