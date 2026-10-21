import Link from "next/link";
import { logoutAction } from "@/app/actions";
import { requireSession } from "@/lib/server/session";
import { MainNav } from "@/components/main-nav";

export default async function ConsoleLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  await requireSession();
  return <div className="app-shell">
    <aside className="sidebar">
      <Link className="brand" href="/provisioning"><span className="brand-mark small">L</span><span>lynx<span className="brand-light">/os</span></span></Link>
      <p className="nav-caption">CENTRAL</p>
      <MainNav />
      <div className="sidebar-bottom"><div className="status-dot" /> Ambiente seguro <form action={logoutAction}><button className="logout-button">Sair ↗</button></form></div>
    </aside>
    <main className="main-area"><header className="topbar"><div><span className="crumb">LYNX OS</span><span className="crumb-separator">/</span><span>Operações</span></div><div className="user-pill"><span className="avatar">L</span> Administrador</div></header>{children}</main>
  </div>;
}
