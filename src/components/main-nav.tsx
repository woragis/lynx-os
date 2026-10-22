"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

export function MainNav() {
  const pathname = usePathname();
  return <nav className="main-nav">
    <Link href="/accounts" aria-current={pathname.startsWith("/accounts") ? "page" : undefined}><span className="nav-icon">⌂</span>Empresas e acessos</Link>
    <Link href="/provisioning" aria-current={pathname === "/provisioning" ? "page" : undefined}><span className="nav-icon">＋</span>Novo cadastro</Link>
    <Link href="/integrations" aria-current={pathname === "/integrations" ? "page" : undefined}><span className="nav-icon">◈</span>SaaS e integrações</Link>
  </nav>;
}
