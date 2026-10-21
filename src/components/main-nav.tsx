"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

export function MainNav() {
  const pathname = usePathname();
  return <nav className="main-nav">
    <Link href="/provisioning" aria-current={pathname === "/provisioning" ? "page" : undefined}><span className="nav-icon">＋</span>Novo cadastro</Link>
    <Link href="/integrations" aria-current={pathname === "/integrations" ? "page" : undefined}><span className="nav-icon">◈</span>Integrações</Link>
  </nav>;
}
