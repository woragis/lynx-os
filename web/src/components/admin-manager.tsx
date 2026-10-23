"use client";

import { useCallback, useEffect, useState } from "react";
import { csrfToken, ensureCsrfToken } from "@/lib/auth-client";

type Admin = {
  id: string;
  email: string;
  displayName: string;
  role: "owner" | "admin" | "operator" | "viewer";
  active: boolean;
  createdAt: string;
  lastLoginAt: string | null;
};

const roleLabels = { owner: "Owner", admin: "Administrador", operator: "Operador", viewer: "Leitura" };

export function AdminManager() {
  const [admins, setAdmins] = useState<Admin[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [message, setMessage] = useState("");
  const [inviteLink, setInviteLink] = useState("");

  const load = useCallback(async () => {
    const response = await fetch("/api/v1/auth/admins", { credentials: "same-origin", cache: "no-store" });
    if (!response.ok) throw new Error("list failed");
    const result = await response.json() as { data: Admin[] };
    setAdmins(result.data);
  }, []);

  useEffect(() => {
    let mounted = true;
    fetch("/api/v1/auth/admins", { credentials: "same-origin", cache: "no-store" })
      .then(async (response) => {
        if (!response.ok) throw new Error("list failed");
        return await response.json() as { data: Admin[] };
      })
      .then((result) => { if (mounted) setAdmins(result.data); })
      .catch(() => { if (mounted) setMessage("Não foi possível carregar os administradores."); })
      .finally(() => { if (mounted) setLoading(false); });
    return () => { mounted = false; };
  }, []);

  async function invite(formData: FormData) {
    setBusy("invite");
    setMessage("");
    setInviteLink("");
    try {
      const csrf = csrfToken() || await ensureCsrfToken();
      const response = await fetch("/api/v1/auth/admins/invitations", {
        method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ email: formData.get("email"), displayName: formData.get("displayName"), role: formData.get("role") }),
      });
      const result = await response.json() as { data?: { token?: string }; error?: { code?: string } };
      if (!response.ok || !result.data?.token) {
        setMessage(result.error?.code === "email_already_used" ? "Já existe uma conta com esse e-mail." : "Não foi possível criar o convite.");
        return;
      }
      setInviteLink(`${window.location.origin}/accept-invitation?token=${encodeURIComponent(result.data.token)}`);
      setMessage("Convite criado. Copie o link; ele aparece apenas agora e expira em 24 horas.");
      await load();
    } catch {
      setMessage("Não foi possível conectar ao backend.");
    } finally {
      setBusy("");
    }
  }

  async function update(admin: Admin, changes: Partial<Pick<Admin, "displayName" | "role" | "active">>) {
    setBusy(admin.id);
    setMessage("");
    try {
      const csrf = csrfToken() || await ensureCsrfToken();
      const response = await fetch(`/api/v1/auth/admins/${encodeURIComponent(admin.id)}`, {
        method: "PATCH", credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({ displayName: changes.displayName ?? admin.displayName, role: changes.role ?? admin.role, active: changes.active ?? admin.active }),
      });
      if (!response.ok) throw new Error("update failed");
      await load();
    } catch {
      setMessage("Não foi possível atualizar esta conta.");
    } finally {
      setBusy("");
    }
  }

  return <>
    <section className="panel admin-invite-panel">
      <div className="section-heading"><span className="step-number">＋</span><div><h2>Convidar administrador</h2><p>O link de ativação aparece uma vez e expira em 24 horas.</p></div></div>
      <form action={invite} className="admin-invite-form">
        <label>Nome<input name="displayName" maxLength={120} required /></label>
        <label>E-mail<input name="email" type="email" maxLength={254} required /></label>
        <label>Papel<select name="role" defaultValue="operator"><option value="admin">Administrador</option><option value="operator">Operador</option><option value="viewer">Leitura</option></select></label>
        <button className="button button-dark" type="submit" disabled={busy !== ""}>{busy === "invite" ? "Criando…" : "Criar convite"}</button>
      </form>
      {message && <p className="security-message" role="status">{message}</p>}
      {inviteLink && <div className="invite-link"><label>Link de ativação<input readOnly value={inviteLink} onFocus={(event) => event.currentTarget.select()} /></label><button className="quiet-button" type="button" onClick={() => navigator.clipboard.writeText(inviteLink)}>Copiar link</button></div>}
    </section>

    <section className="panel admin-list-panel">
      <div className="panel-heading"><div><h2>Contas administrativas</h2><p>Desativar uma conta revoga todas as sessões abertas.</p></div><span className="count-badge">{admins.length} contas</span></div>
      {loading ? <p>Carregando contas…</p> : <div className="admin-list">{admins.map((admin) => <article className="admin-row" key={admin.id}>
        <div className="admin-identity"><strong>{admin.displayName}</strong><span>{admin.email}</span><small>{admin.lastLoginAt ? `Último acesso ${new Date(admin.lastLoginAt).toLocaleString("pt-BR")}` : "Ainda não entrou"}</small></div>
        <span className={`admin-state ${admin.active ? "active" : "inactive"}`}>{admin.active ? "Ativa" : "Desativada"}</span>
        <select aria-label={`Papel de ${admin.displayName}`} value={admin.role} disabled={admin.role === "owner" || busy !== ""} onChange={(event) => update(admin, { role: event.target.value as Admin["role"] })}>
          {Object.entries(roleLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </select>
        {admin.role === "owner" ? <span className="owner-lock">Owner protegido</span> : <button className="quiet-button" type="button" disabled={busy !== ""} onClick={() => update(admin, { active: !admin.active })}>{busy === admin.id ? "Salvando…" : admin.active ? "Desativar" : "Reativar"}</button>}
      </article>)}</div>}
    </section>
  </>;
}
