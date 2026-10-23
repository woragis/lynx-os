import { AdminManager } from "@/components/admin-manager";
import { requireRoles } from "@/lib/server/session";

export default async function TeamPage() {
  await requireRoles(["owner"]);
  return <div className="page-wrap">
    <div className="page-heading"><div><p className="eyebrow">ACESSO INTERNO</p><h1>Equipe</h1><p className="muted">Convide pessoas e controle o acesso administrativo.</p></div><span className="secure-tag"><span /> ACESSO DO OWNER</span></div>
    <AdminManager />
  </div>;
}
