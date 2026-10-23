import { randomUUID } from "node:crypto";
import { provisionAction, retryProvisionAction } from "@/app/actions";
import { getOperation, listIntegrations } from "@/lib/server/provisioning";
import { requireSession } from "@/lib/server/session";

type ProvisioningAccess = { url: string; email: string; invitationState?: string };
type CreatedResource = { id: string; state: string };
type BootstrapOutcome = { account?: CreatedResource; initialCustomer?: CreatedResource; initialUser?: CreatedResource; invitation?: { state?: string } };

function responseData(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || !("data" in value)) return null;
  const data = value.data;
  return data && typeof data === "object" ? data as Record<string, unknown> : null;
}

function getProvisioningAccess(value: unknown): ProvisioningAccess | null {
  const data = responseData(value);
  if (!data || !("login" in data)) return null;
  const login = data.login;
  if (!login || typeof login !== "object" || !("url" in login) || !("email" in login)) return null;
  if (typeof login.url !== "string" || typeof login.email !== "string") return null;
  try {
    const url = new URL(login.url);
    if (url.protocol !== "https:" && !(url.protocol === "http:" && url.hostname === "localhost")) return null;
  } catch {
    return null;
  }
  const invitation = "invitation" in data ? data.invitation : null;
  const invitationState = invitation && typeof invitation === "object" && "state" in invitation && typeof invitation.state === "string"
    ? invitation.state
    : undefined;
  return { url: login.url, email: login.email, invitationState };
}

function getBootstrapOutcome(value: unknown): BootstrapOutcome | null {
  const record = responseData(value);
  if (!record) return null;
  const resource = (key: string): CreatedResource | undefined => {
    const item = record[key];
    if (!item || typeof item !== "object") return undefined;
    const candidate = item as Record<string, unknown>;
    return typeof candidate.id === "string" && typeof candidate.state === "string"
      ? { id: candidate.id, state: candidate.state }
      : undefined;
  };
  const invitation = record.invitation && typeof record.invitation === "object" ? record.invitation as { state?: string } : undefined;
  return { account: resource("account"), initialCustomer: resource("initialCustomer"), initialUser: resource("initialUser"), invitation };
}

export default async function ProvisioningPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  const params = await searchParams;
  await requireSession();
  const integrations = (await listIntegrations()).filter((item) => item.active && !item.revokedAt).map(({ id, name }) => ({ id, name }));
  const integrationId = params.integrationId ?? "";
  let request: { idempotencyKey: string; status: string; response?: unknown } | null = null;
  if (params.request && integrationId) { try { const operation = await getOperation(integrationId, params.request); request = { idempotencyKey: operation.idempotencyKey, status: operation.status.toUpperCase(), response: operation.response }; } catch { request = null; } }
  const result: unknown = request?.response ?? null;
  const access = getProvisioningAccess(result);
  const outcome = getBootstrapOutcome(result);
  return <div className="page-wrap">
    <div className="page-heading"><div><p className="eyebrow">PROVISIONAMENTO CENTRAL</p><h1>Novo cadastro</h1><p className="muted">Uma solicitação. Empresa, cliente e acesso inicial no sistema escolhido.</p></div><div className="secure-tag"><span /> ROTA PADRÃO · V1</div></div>
    {params.erro && <p className="error-banner" role="alert">{params.erro === "conexao" ? "Não foi possível concluir a chamada. Você pode tentar novamente com segurança." : "Confira os campos obrigatórios e a integração ativa."}</p>}
    {request && <section className="result-card"><div className="result-icon">{request.status === "COMPLETED" ? "✓" : "!"}</div><div><p className="eyebrow">RESULTADO DO SAAS · {request.status}</p><h2>{request.status === "COMPLETED" ? "Cadastro processado" : request.status === "FAILED" ? "Cadastro aguardando resposta" : "O SaaS recusou o cadastro"}</h2>{outcome && <ul className="result-resources">{outcome.account && <li><span>Empresa</span><strong>{outcome.account.state === "created" ? "Criada" : "Localizada"}</strong></li>}{outcome.initialCustomer && <li><span>Cliente inicial</span><strong>{outcome.initialCustomer.state === "created" ? "Criado" : "Localizado"}</strong></li>}{outcome.initialUser && <li><span>Usuário responsável</span><strong>{outcome.initialUser.state === "created" ? "Criado" : "Já tinha acesso"}</strong></li>}</ul>}{!result && <p className="muted">Sem resposta armazenada. Se houve timeout, reenvie com a mesma chave de idempotência.</p>}{access && <div className="provisioning-access"><p><strong>Acesso ao sistema</strong><br /><a href={access.url} target="_blank" rel="noreferrer">Abrir tela de login ↗</a><br />Usuário: {access.email}</p><p>{access.invitationState === "sent" ? "Convite enviado para criar a senha." : access.invitationState === "already_registered" ? "Conta existente vinculada; use a senha já cadastrada." : "Convite não enviado. O cadastro foi criado, mas é necessário configurar o e-mail para definir a senha."}</p></div>}{request.status === "FAILED" && <form action={retryProvisionAction}><input type="hidden" name="idempotencyKey" value={request.idempotencyKey} /><input type="hidden" name="integrationId" value={integrationId} /><button className="button button-dark" type="submit">Tentar novamente com segurança ↗</button></form>}</div></section>}
    {integrations.length === 0 ? <div className="empty-state wide-empty"><span className="empty-mark">◈</span><h2>Conecte seu primeiro SaaS</h2><p>Cadastre domínio e chave de provisionamento antes de iniciar.</p><a className="button button-dark" href="/integrations">Ir para integrações ↗</a></div> : <form action={provisionAction} className="provision-form">
      <input type="hidden" name="idempotencyKey" value={randomUUID()} />
      <section className="panel form-section"><div className="section-heading"><span className="step-number">01</span><div><h2>Sistema e empresa</h2><p>Conta principal do SaaS (tenant).</p></div></div>
        <div className="field-grid"><label className="field-span">Sistema<select name="integrationId" required defaultValue=""><option value="" disabled>Selecione um SaaS conectado</option>{integrations.map((item) => <option value={item.id} key={item.id}>{item.name}</option>)}</select></label>
          <label>Nome do escritório<input name="accountName" required placeholder="Contabilidade Exemplo" /></label><label>CNPJ do escritório<input name="accountTaxId" required inputMode="numeric" placeholder="00.000.000/0001-00" /></label><label>E-mail do escritório<input name="accountEmail" type="email" required placeholder="contato@escritorio.com.br" /></label><label>Endereço completo<input name="accountAddress" required placeholder="Rua, número, cidade e UF" /></label></div>
      </section>
      <section className="panel form-section"><div className="section-heading"><span className="step-number">02</span><div><h2>Primeiro cliente</h2><p>Registro da carteira dentro do escritório.</p></div></div>
        <div className="field-grid"><label>Nome fantasia<input name="customerTradeName" required placeholder="Empresa Cliente" /></label><label>Razão social <span className="optional">(opcional)</span><input name="customerLegalName" placeholder="Empresa Cliente Ltda." /></label><label>CNPJ ou CPF<input name="customerTaxId" required inputMode="numeric" placeholder="00.000.000/0001-00" /></label><label>E-mail <span className="optional">(opcional)</span><input name="customerEmail" type="email" placeholder="financeiro@cliente.com.br" /></label><label>Telefone <span className="optional">(opcional)</span><input name="customerPhone" type="tel" placeholder="(11) 99999-9999" /></label><label>Regime tributário<input name="customerTaxRegime" required placeholder="Simples Nacional" /></label>
          <label>Tipo de relação<select name="relationshipType" required defaultValue=""><option value="" disabled>Selecione</option><option value="Recorrente">Recorrente</option><option value="Avulso">Avulso</option></select></label><label>Status<select name="customerStatus" required defaultValue=""><option value="" disabled>Selecione</option><option value="Ativo">Ativo</option><option value="Inativo">Inativo</option></select></label></div>
      </section>
      <section className="panel form-section"><div className="section-heading"><span className="step-number">03</span><div><h2>Usuário responsável</h2><p>Receberá um convite para definir a própria senha.</p></div></div>
        <div className="field-grid"><label>Nome completo<input name="userName" required placeholder="Nome da pessoa responsável" /></label><label>E-mail de acesso<input name="userEmail" type="email" required placeholder="pessoa@escritorio.com.br" /></label><label>Telefone <span className="optional">(opcional)</span><input name="userPhone" type="tel" placeholder="(11) 99999-9999" /></label><div className="role-preview"><span className="role-icon">♙</span><div><strong>Proprietário · Owner</strong><span>Acesso inicial à empresa</span></div></div></div>
      </section>
      <div className="submit-row"><p>Antes de enviar, revise os dados. A ação cria registros no sistema escolhido.</p><button className="button button-dark" type="submit">Criar cadastro <span aria-hidden>↗</span></button></div>
    </form>}
  </div>;
}
