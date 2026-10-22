import Link from "next/link";
import Image from "next/image";
import { logoutAction } from "@/app/actions";
import { requireSession } from "@/lib/server/session";
import { MainNav } from "@/components/main-nav";

export default async function ConsoleLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  await requireSession();
  return <div className="app-shell">
    <aside className="sidebar">
      <Link className="brand" href="/provisioning"><Image className="brand-logo" src="/lynx.jpg" alt="" width={38} height={38} /><span>LYNX<span className="brand-light"> / OS</span></span></Link>
      <p className="nav-caption">CENTRAL</p>
      <MainNav />
      <div className="sidebar-bottom"><div className="status-dot" /> Ambiente seguro</div>
    </aside>
    <main className="main-area"><header className="topbar"><div><span className="crumb">LYNX OS</span><span className="crumb-separator">/</span><span>Operações</span></div><div className="user-pill"><span className="avatar">L</span> Administrador<form action={logoutAction}><button className="topbar-logout">Sair ↗</button></form></div></header>{children}</main>
  </div>;
}
