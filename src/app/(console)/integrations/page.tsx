import { addIntegrationAction, revokeIntegrationAction, rotateIntegrationKeyAction, toggleIntegrationAction } from "@/app/actions";
import { db } from "@/lib/server/db";

export default async function IntegrationsPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  const params = await searchParams;
  const integrations = await db.integration.findMany({ orderBy: { createdAt: "desc" }, select: { id: true, name: true, baseUrl: true, active: true, revokedAt: true } });
  return <div className="page-wrap">
    <div className="page-heading"><div><p className="eyebrow">ECOSSISTEMA LYNX</p><h1>Integrações</h1><p className="muted">Conecte cada SaaS Lynx pelo contrato único de provisionamento.</p></div><span className="count-badge">{integrations.length.toString().padStart(2, "0")} sistemas</span></div>
    {params.erro && <p className="error-banner" role="alert">Não foi possível salvar: confira os campos, o domínio HTTPS e se o nome já existe.</p>}
    {params.salvo && <p className="success-banner">Integração salva. A chave não será exibida novamente.</p>}
    {params.rotacionada && <p className="success-banner">Chave validada e substituída no Lynx OS. Revogue a chave antiga no painel do SaaS.</p>}
    {params.revogada && <p className="success-banner">Chave apagada do Lynx OS e integração bloqueada. Revogue-a também no SaaS para invalidá-la remotamente.</p>}
    <section className="integration-grid">
      <div className="panel integration-form-panel"><div className="panel-title"><div><span className="panel-icon">＋</span><h2>Adicionar sistema</h2></div></div><p className="muted small-text">Use uma chave com apenas <code>provisioning:read</code> e <code>provisioning:write</code>. Ela é cifrada no servidor.</p>
        <form action={addIntegrationAction} className="stack-form">
          <label>Nome do sistema<input name="name" placeholder="EasyBill" required /></label>
          <label>URL base<input name="baseUrl" type="url" placeholder="https://api.easybill.com" required /></label>
          <label>API key<input name="apiKey" type="password" autoComplete="new-password" placeholder="Chave de provisionamento" required /></label>
          <button className="button button-dark" type="submit">Salvar integração <span aria-hidden>↗</span></button>
        </form>
      </div>
      <div className="integration-list">{integrations.length === 0 ? <div className="empty-state"><span className="empty-mark">◈</span><h2>Nenhum sistema conectado</h2><p>Adicione um SaaS para liberar o primeiro cadastro centralizado.</p></div> : integrations.map((item) => <article className="integration-card" key={item.id}>
        <div className="integration-logo">{item.name.slice(0, 1).toUpperCase()}</div><div className="integration-details"><div className="integration-name-row"><h2>{item.name}</h2><span className={`state-chip ${item.revokedAt ? "paused" : item.active ? "active" : "paused"}`}>{item.revokedAt ? "Revogada localmente" : item.active ? "Ativa" : "Pausada"}</span></div><p>{item.baseUrl}</p><span className="key-hint">{item.revokedAt ? "API key apagada do Lynx OS" : "API key •••••••• · cifrada"}</span></div>
        <div className="integration-actions">{!item.revokedAt && <><form action={toggleIntegrationAction}><input type="hidden" name="id" value={item.id} /><button className="quiet-button" type="submit">{item.active ? "Pausar" : "Ativar"}</button></form><details className="rotate-details"><summary>Rotacionar</summary><form action={rotateIntegrationKeyAction}><input type="hidden" name="id" value={item.id} /><input type="password" name="apiKey" autoComplete="new-password" placeholder="Nova API key" required /><button type="submit">Validar e trocar</button></form></details><details className="revoke-details"><summary>Revogar</summary><form action={revokeIntegrationAction}><input type="hidden" name="id" value={item.id} /><p>Apaga a chave local e bloqueia novas chamadas. Confirme também a revogação no SaaS.</p><button type="submit">Confirmar revogação</button></form></details></>}</div>
      </article>)}</div>
    </section>
    <div className="security-note"><span>⌑</span><p><strong>Credenciais protegidas.</strong> As chaves são armazenadas com AES-256-GCM. Revogar apaga a chave daqui e bloqueia chamadas; revogue-a também no painel do SaaS para invalidá-la remotamente. Mantenha <code>LYNX_ENCRYPTION_KEY</code> fora do banco e do repositório.</p></div>
  </div>;
}
