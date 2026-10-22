import { addIntegrationAction, editIntegrationAction, revokeIntegrationAction, rotateIntegrationKeyAction, toggleIntegrationAction } from "@/app/actions";
import { db } from "@/lib/server/db";

export default async function IntegrationsPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  const params = await searchParams;
  const integrations = await db.integration.findMany({ orderBy: { createdAt: "desc" }, select: { id: true, name: true, baseUrl: true, appUrl: true, active: true, revokedAt: true } });
  return <div className="page-wrap">
    <div className="page-heading"><div><p className="eyebrow">ECOSSISTEMA LYNX</p><h1>Integrações</h1><p className="muted">Conecte cada SaaS Lynx pelo contrato único de provisionamento.</p></div><span className="count-badge">{integrations.length.toString().padStart(2, "0")} sistemas</span></div>
    {params.erro && <p className="error-banner" role="alert">{params.erro === "conexao" ? "Não conseguimos validar o contrato do SaaS. Confira o endereço da API e a API key." : params.erro === "app-url" ? "Informe uma URL HTTPS válida para abrir o painel do SaaS." : params.erro === "chave" ? "A nova chave não tem acesso ao contrato do SaaS ou a conexão falhou." : params.erro === "rotacao" ? "Informe uma chave válida para uma integração ativa." : params.erro === "nao-encontrada" ? "A integração não existe ou já foi revogada." : params.erro === "revogada" ? "Uma chave revogada não pode ser ativada novamente; cadastre uma nova integração." : params.erro === "https" || params.erro === "url" ? "Use uma URL HTTPS válida, sem usuário, senha, query ou fragmento." : params.erro === "duplicada" ? "Já existe um SaaS com esse nome. Escolha outro nome." : "Confira os campos obrigatórios e tente novamente."}</p>}
    {params.salvo && <p className="success-banner">Integração salva. A chave não será exibida novamente.</p>}
    {params.atualizado && <p className="success-banner">Dados da integração atualizados.</p>}
    {params.rotacionada && <p className="success-banner">Chave validada e substituída no Lynx OS. Revogue a chave antiga no painel do SaaS.</p>}
    {params.revogada && <p className="success-banner">Chave apagada do Lynx OS e integração bloqueada. Revogue-a também no SaaS para invalidá-la remotamente.</p>}
    <section className="integration-grid">
      <div className="panel integration-form-panel"><div className="panel-title"><div><span className="panel-icon">＋</span><h2>Adicionar sistema</h2></div></div><p className="muted small-text">Use uma chave com apenas <code>provisioning:read</code> e <code>provisioning:write</code>. Ela é cifrada no servidor.</p>
        <form action={addIntegrationAction} className="stack-form">
          <label>Nome do sistema<input name="name" placeholder="EasyBill" required /></label>
          <label>URL da API<input name="baseUrl" type="url" placeholder="https://easybill.lynx.com.br" required /></label>
          <label>URL do painel <span className="optional">(opcional)</span><input name="appUrl" type="url" placeholder="https://easybill.lynx.com.br/login" /></label>
          <label>API key<input name="apiKey" type="password" autoComplete="new-password" placeholder="Chave de provisionamento" required /></label>
          <button className="button button-dark" type="submit">Salvar integração <span aria-hidden>↗</span></button>
        </form>
      </div>
      <div className="integration-list">{integrations.length === 0 ? <div className="empty-state"><span className="empty-mark">◈</span><h2>Nenhum sistema conectado</h2><p>Adicione um SaaS para liberar o primeiro cadastro centralizado.</p></div> : integrations.map((item) => <article className="integration-card" key={item.id}>
        <div className="integration-logo">{item.name.slice(0, 1).toUpperCase()}</div><div className="integration-details"><div className="integration-name-row"><h2>{item.name}</h2><span className={`state-chip ${item.revokedAt ? "paused" : item.active ? "active" : "paused"}`}>{item.revokedAt ? "Revogada localmente" : item.active ? "Ativa" : "Pausada"}</span></div><p>{item.baseUrl}</p><span className="key-hint">{item.revokedAt ? "API key apagada do Lynx OS" : "API key protegida · cifrada"}</span></div>
        <div className="integration-actions">{!item.revokedAt && <>
          {item.appUrl && <a className="quiet-button" href={item.appUrl} target="_blank" rel="noreferrer">Abrir painel ↗</a>}
          <details className="edit-details"><summary>Editar SaaS</summary><form action={editIntegrationAction}><input type="hidden" name="id" value={item.id} /><label>Nome<input name="name" defaultValue={item.name} required maxLength={120} /></label><label>URL da API<input name="baseUrl" type="url" defaultValue={item.baseUrl} required /></label><label>URL do painel<input name="appUrl" type="url" defaultValue={item.appUrl ?? ""} placeholder="https://..." /></label><button type="submit">Salvar alterações</button></form></details>
          <form action={toggleIntegrationAction}><input type="hidden" name="id" value={item.id} /><button className="quiet-button" type="submit">{item.active ? "Pausar" : "Ativar"}</button></form><details className="rotate-details"><summary>Trocar chave</summary><form action={rotateIntegrationKeyAction}><input type="hidden" name="id" value={item.id} /><input type="password" name="apiKey" autoComplete="new-password" placeholder="Nova API key" required /><button type="submit">Validar e trocar</button></form></details><details className="revoke-details"><summary>Revogar</summary><form action={revokeIntegrationAction}><input type="hidden" name="id" value={item.id} /><p>Apaga a chave local e bloqueia chamadas. Revogue também no painel do SaaS.</p><button type="submit">Confirmar revogação</button></form></details>
        </>}</div>
      </article>)}</div>
    </section>
    <div className="security-note"><span>⌑</span><p><strong>Credenciais protegidas.</strong> As chaves são armazenadas com AES-256-GCM. Revogar apaga a chave daqui e bloqueia chamadas; revogue-a também no painel do SaaS para invalidá-la remotamente. Mantenha <code>LYNX_ENCRYPTION_KEY</code> fora do banco e do repositório.</p></div>
  </div>;
}
