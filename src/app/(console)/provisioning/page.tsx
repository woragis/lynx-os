import { randomUUID } from "node:crypto";
import { provisionAction, retryProvisionAction } from "@/app/actions";
import { decryptSecret } from "@/lib/server/crypto";
import { db } from "@/lib/server/db";

export default async function ProvisioningPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  const params = await searchParams;
  const integrations = await db.integration.findMany({ where: { active: true }, orderBy: { name: "asc" }, select: { id: true, name: true } });
  const request = params.request ? await db.provisioningRecord.findUnique({ where: { idempotencyKey: params.request }, select: { idempotencyKey: true, status: true, responseCiphertext: true, responseIv: true, responseAuthTag: true } }) : null;
  let result: unknown = null;
  try { result = request?.responseCiphertext && request.responseIv && request.responseAuthTag ? JSON.parse(decryptSecret(request.responseCiphertext, request.responseIv, request.responseAuthTag)) : null; } catch { result = "Não foi possível abrir a resposta armazenada."; }
  return <div className="page-wrap">
    <div className="page-heading"><div><p className="eyebrow">PROVISIONAMENTO CENTRAL</p><h1>Novo cadastro</h1><p className="muted">Uma solicitação. Empresa, cliente e acesso inicial no sistema escolhido.</p></div><div className="secure-tag"><span /> ROTA PADRÃO · V1</div></div>
    {params.erro && <p className="error-banner" role="alert">{params.erro === "conexao" ? "Não foi possível concluir a chamada. Você pode tentar novamente com segurança." : "Confira os campos obrigatórios e a integração ativa."}</p>}
    {request && <section className="result-card"><div className="result-icon">{request.status === "COMPLETED" ? "✓" : "!"}</div><div><p className="eyebrow">RESPOSTA DO SISTEMA · {request.status}</p><h2>Resultado do provisionamento</h2><pre>{result ? JSON.stringify(result, null, 2) : "Sem resposta armazenada. Se houve timeout, reenvie com a mesma chave de idempotência."}</pre>{request.status === "RETRYABLE" && <form action={retryProvisionAction}><input type="hidden" name="idempotencyKey" value={request.idempotencyKey} /><button className="button button-dark" type="submit">Tentar novamente com segurança ↗</button></form>}</div></section>}
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
