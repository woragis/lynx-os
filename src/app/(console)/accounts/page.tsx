import { randomUUID } from "node:crypto";
import Link from "next/link";
import { manageResourceAction, retryResourceAction } from "@/app/actions";
import { decryptSecret } from "@/lib/server/crypto";
import { db } from "@/lib/server/db";
import { discoverCapabilities, listResources, type ContractCapabilities } from "@/lib/server/provisioning";

type SearchParams = Promise<Record<string, string | undefined>>;
type Resource = Record<string, unknown> & { id: string; resourceType: string; lifecycle?: Record<string, unknown> };

const resourceLabels: Record<string, string> = { account: "Empresas", customer: "Clientes", user: "Usuários" };
const fieldLabels: Record<string, string> = {
  name: "Nome", taxId: "CNPJ / CPF", email: "E-mail", address: "Endereço",
  tradeName: "Nome fantasia", legalName: "Razão social", phone: "Telefone",
  taxRegime: "Regime tributário", relationshipType: "Tipo de relação", status: "Status",
};

function getItems(value: unknown): Resource[] {
  if (!value || typeof value !== "object" || !("data" in value)) return [];
  const data = value.data;
  if (!data || typeof data !== "object" || !("items" in data) || !Array.isArray(data.items)) return [];
  return data.items.filter((item): item is Resource => Boolean(item && typeof item === "object" && "id" in item && typeof item.id === "string"));
}

function getNextCursor(value: unknown) {
  if (!value || typeof value !== "object" || !("data" in value)) return null;
  const data = value.data;
  if (!data || typeof data !== "object" || !("page" in data)) return null;
  const page = data.page;
  return page && typeof page === "object" && "nextCursor" in page && typeof page.nextCursor === "string" ? page.nextCursor : null;
}

function asString(value: unknown) { return typeof value === "string" ? value : ""; }
function lifecycleLabel(value: unknown) {
  if (!value || typeof value !== "object") return "Ativo";
  const lifecycle = value as Record<string, unknown>;
  return lifecycle.state === "hibernated" ? "Hibernado" : lifecycle.state === "purged" ? "Purgado" : lifecycle.state === "inactive" ? "Suspenso" : "Ativo";
}

function resultSummary(value: unknown) {
  if (!value || typeof value !== "object") return "O SaaS não devolveu detalhes de resultado.";
  if ("error" in value) {
    const error = value.error;
    if (error && typeof error === "object" && "message" in error && typeof error.message === "string") return error.message;
    return "O SaaS recusou a operação. Confira o cadastro e tente novamente.";
  }
  const data = "data" in value && value.data && typeof value.data === "object" ? value.data as Record<string, unknown> : null;
  const status = data && typeof data.state === "string" ? data.state : "concluída";
  const id = data && typeof data.id === "string" ? ` · referência ${data.id}` : "";
  return `Operação ${status}${id}.`;
}

export default async function AccountsPage({ searchParams }: { searchParams: SearchParams }) {
  const params = await searchParams;
  const integrations = await db.integration.findMany({ where: { active: true, revokedAt: null }, orderBy: { name: "asc" }, select: { id: true, name: true, appUrl: true } });
  const integrationId = params.integrationId && integrations.some((item) => item.id === params.integrationId)
    ? params.integrationId
    : integrations[0]?.id ?? "";
  const resourceType = ["account", "customer", "user"].includes(params.resourceType ?? "") ? params.resourceType! : "account";
  const lifecycle = ["active", "hibernated", "all"].includes(params.lifecycle ?? "") ? params.lifecycle! : "active";
  let capabilities: ContractCapabilities | null = null;
  let items: Resource[] = [];
  let accounts: Resource[] = [];
  let nextCursor: string | null = null;
  let requestRecord: { status: string; responseCiphertext: string | null; responseIv: string | null; responseAuthTag: string | null } | null = null;
  let requestResult: unknown = null;
  let listError = "";

  if (integrationId) {
    try {
      capabilities = await discoverCapabilities(integrationId);
      if (resourceType === "account") {
        const result = await listResources(integrationId, "account", { lifecycle, search: params.search, cursor: params.cursor });
        items = getItems(result);
        nextCursor = getNextCursor(result);
        accounts = items;
      } else {
        const [accountResult, resourceResult] = await Promise.all([
          listResources(integrationId, "account", { lifecycle: "all" }),
          params.accountId ? listResources(integrationId, resourceType, { accountId: params.accountId, lifecycle, search: params.search, cursor: params.cursor }) : Promise.resolve(null),
        ]);
        accounts = getItems(accountResult);
        items = getItems(resourceResult);
        nextCursor = getNextCursor(resourceResult);
      }
    } catch (error) {
      listError = error instanceof Error ? error.message : "Não foi possível consultar este SaaS.";
    }
  }

  if (params.request) {
    requestRecord = await db.provisioningRecord.findUnique({ where: { idempotencyKey: params.request }, select: { status: true, responseCiphertext: true, responseIv: true, responseAuthTag: true } });
    try {
      if (requestRecord?.responseCiphertext && requestRecord.responseIv && requestRecord.responseAuthTag) {
        requestResult = JSON.parse(decryptSecret(requestRecord.responseCiphertext, requestRecord.responseIv, requestRecord.responseAuthTag));
      }
    } catch { requestResult = null; }
  }

  const resourceCapabilities = capabilities?.resources[resourceType];
  const operations = resourceCapabilities?.operations ?? [];
  const editable = Array.isArray(resourceCapabilities?.fields?.editable) ? resourceCapabilities.fields.editable.filter((field): field is string => typeof field === "string") : [];
  const requiredFields = Array.isArray(resourceCapabilities?.fields?.required) ? resourceCapabilities.fields.required.filter((field): field is string => typeof field === "string") : [];
  const selectedAccount = accounts.find((item) => item.id === params.accountId);
  const selectedIntegration = integrations.find((item) => item.id === integrationId);
  const unsupportedReason = !resourceCapabilities
    ? "Este SaaS ainda não anuncia este tipo de recurso no contrato v1."
    : !operations.includes("list")
      ? "Este SaaS ainda não permite listar este tipo de recurso."
      : "";

  return <div className="page-wrap">
    <div className="page-heading"><div><p className="eyebrow">OPERAÇÃO DOS SAAS</p><h1>Empresas e acessos</h1><p className="muted">Encontre e mantenha empresas, clientes e usuários cadastrados em cada sistema.</p></div><Link className="button button-dark" href="/provisioning">＋ Novo cadastro</Link></div>

    {params.erro && <p className="error-banner" role="alert">{params.erro === "confirmacao" ? "Digite PURGAR exatamente para confirmar a exclusão permanente." : params.erro === "integracao" ? "A operação não foi concluída. Confira a integração e tente novamente." : "Não foi possível executar essa solicitação. Confira os dados e tente de novo."}</p>}
    {params.result && <p className={params.result === "COMPLETED" ? "success-banner" : "error-banner"}>{params.result === "COMPLETED" ? "Alteração concluída no SaaS." : `Solicitação ${params.result.toLocaleLowerCase()}. Consulte o resultado abaixo.`}</p>}
    {requestRecord && <section className="manager-result"><div><strong>{requestRecord.status === "RETRYABLE" ? "Aguardando nova tentativa" : requestRecord.status === "COMPLETED" ? "Resultado da solicitação" : "O SaaS devolveu uma validação"}</strong><p>{requestRecord.status === "RETRYABLE" ? "O SaaS não confirmou a conclusão. A mesma solicitação pode ser reenviada sem duplicar a operação." : resultSummary(requestResult)}</p></div>{requestRecord.status === "RETRYABLE" && <form action={retryResourceAction}><input type="hidden" name="idempotencyKey" value={params.request} /><button className="quiet-button" type="submit">Tentar novamente</button></form>}</section>}

    {integrations.length === 0 ? <div className="empty-state manager-empty"><span className="empty-mark">◈</span><h2>Conecte um SaaS para começar</h2><p>Depois da conexão, os registros disponíveis aparecem aqui.</p><Link className="button button-dark" href="/integrations">Configurar SaaS ↗</Link></div> : <>
      {listError && <p className="error-banner" role="alert">{listError}</p>}
      <section className="panel manager-toolbar">
        <form action="/accounts" method="get" className="manager-filters">
          <label>Sistema<select name="integrationId" defaultValue={integrationId}>{integrations.map((item) => <option value={item.id} key={item.id}>{item.name}</option>)}</select></label>
          <label>Tipo<select name="resourceType" defaultValue={resourceType}>{Object.entries(resourceLabels).map(([value, label]) => <option value={value} key={value}>{label}</option>)}</select></label>
          {resourceType !== "account" && <label>Empresa<select name="accountId" defaultValue={params.accountId ?? ""}><option value="">Escolha uma empresa</option>{accounts.map((item) => <option value={item.id} key={item.id}>{asString(item.name)}</option>)}</select></label>}
          <label>Situação<select name="lifecycle" defaultValue={lifecycle}><option value="active">Ativos</option><option value="hibernated">Hibernados</option><option value="all">Todos</option></select></label>
          <label className="manager-search">Buscar<input type="search" name="search" defaultValue={params.search ?? ""} placeholder="Nome, documento ou e-mail" /></label>
          <button className="quiet-button" type="submit">Filtrar</button>
        </form>
        <div className="manager-context"><span>{resourceLabels[resourceType]}</span><strong>{selectedAccount ? asString(selectedAccount.name) : resourceType === "account" ? "Todas as empresas" : "Selecione uma empresa"}</strong>{capabilities && (selectedIntegration?.appUrl ? <a href={selectedIntegration.appUrl} target="_blank" rel="noreferrer">Abrir {capabilities.service.name} ↗</a> : <span>{capabilities.service.name}</span>)}</div>
      </section>

      {!capabilities && !listError && <div className="empty-state manager-empty"><span className="empty-mark">!</span><h2>O SaaS não respondeu</h2><p>Verifique a URL da API e a chave em Integrações.</p><Link href="/integrations">Revisar integração</Link></div>}
      {capabilities && unsupportedReason && <div className="empty-state manager-empty"><span className="empty-mark">◈</span><h2>{resourceLabels[resourceType]} ainda não disponíveis</h2><p>{unsupportedReason} O Lynx OS não vai inventar campos nem enviar uma operação que o SaaS não oferece.</p>{resourceType === "account" && <Link className="button button-dark" href="/provisioning">Criar conta com cadastro inicial ↗</Link>}</div>}
      {capabilities && !unsupportedReason && resourceType !== "account" && !params.accountId && <div className="empty-state manager-empty"><span className="empty-mark">⌕</span><h2>Escolha uma empresa</h2><p>Clientes e usuários são sempre exibidos dentro da empresa selecionada.</p></div>}
      {capabilities && !listError && !unsupportedReason && (resourceType === "account" || params.accountId) && <section className="resource-list">
        <div className="resource-list-heading"><div><h2>{resourceLabels[resourceType]}</h2><p>{items.length} registro(s) nesta página · {lifecycle === "active" ? "itens ativos" : lifecycle === "hibernated" ? "itens hibernados" : "todos os estados"}</p></div>{resourceType !== "account" && !operations.includes("create") && <span className="support-chip">Criação avulsa indisponível no SaaS</span>}</div>
        {items.length === 0 ? <div className="empty-state"><h2>Nenhum registro encontrado</h2><p>Tente outro filtro ou confira a empresa selecionada.</p></div> : items.map((item) => {
          const lifecycleValue = item.lifecycle as Record<string, unknown> | undefined;
          const state = lifecycleLabel(lifecycleValue);
          const rowTitle = asString(item.name || item.tradeName || item.legalName || item.email) || "Registro sem nome";
          const purge = resourceCapabilities?.purge;
          const purgeDate = typeof lifecycleValue?.purgeAfter === "string" ? new Date(lifecycleValue.purgeAfter) : null;
          const purgeDue = Boolean(purgeDate && !Number.isNaN(purgeDate.getTime()) && lifecycleValue?.restorable === false);
          const parentHibernated = resourceType !== "account" && lifecycleLabel(selectedAccount?.lifecycle) === "Hibernado";
          return <article className="resource-card" key={item.id}>
            <div className="resource-card-top"><div className="resource-avatar">{rowTitle.slice(0, 1).toLocaleUpperCase()}</div><div className="resource-main"><div className="resource-title"><h3>{rowTitle}</h3><span className={`state-chip ${state === "Ativo" ? "active" : "paused"}`}>{state}</span></div><p>{resourceType === "account" ? `${asString(item.taxId)} · ${asString(item.email)}` : resourceType === "customer" ? `${asString(item.taxId)} · ${asString(item.email)}` : `${asString(item.email)} · ${asString(item.role)}`}</p><small>{resourceType === "account" ? asString(item.address) : asString(item.phone)}</small></div></div>
            <div className="resource-controls">
              {operations.includes("update") && state === "Ativo" && !parentHibernated && <details className="resource-edit"><summary>Editar dados</summary><form action={manageResourceAction}><input type="hidden" name="integrationId" value={integrationId} /><input type="hidden" name="resourceType" value={resourceType} /><input type="hidden" name="resourceId" value={item.id} /><input type="hidden" name="accountId" value={resourceType === "account" ? "" : String(item.accountId ?? params.accountId ?? "")} /><input type="hidden" name="operation" value="resource.update" /><input type="hidden" name="idempotencyKey" value={randomUUID()} />
                {resourceType === "user" && <p className="identity-notice">Nome, e-mail e telefone são da identidade global e podem refletir em outras empresas.</p>}
                <div className="resource-edit-grid">{editable.map((field) => {
                  const fieldValue = item[field];
                  if (typeof fieldValue !== "string") return null;
                  const enumOptions = resourceCapabilities?.fields?.[field];
                  return <label key={field}>{fieldLabels[field] ?? field}<>{Array.isArray(enumOptions) ? <select name={`change.${field}`} defaultValue={fieldValue}>{enumOptions.map((option) => <option value={String(option)} key={String(option)}>{String(option)}</option>)}</select> : <input name={`change.${field}`} type={field === "email" ? "email" : "text"} defaultValue={fieldValue} maxLength={240} required={requiredFields.includes(field)} />}</></label>;
                })}</div><button className="button button-dark" type="submit">Salvar alterações</button></form></details>}
              {state === "Hibernado" && operations.includes("reactivate") && !parentHibernated && <form action={manageResourceAction}><input type="hidden" name="integrationId" value={integrationId} /><input type="hidden" name="resourceType" value={resourceType} /><input type="hidden" name="resourceId" value={item.id} /><input type="hidden" name="accountId" value={resourceType === "account" ? "" : String(item.accountId ?? params.accountId ?? "")} /><input type="hidden" name="operation" value="resource.reactivate" /><input type="hidden" name="idempotencyKey" value={randomUUID()} /><button className="quiet-button" type="submit">Reativar</button></form>}
              {parentHibernated && <span className="support-chip">Reative primeiro a empresa</span>}
              {state === "Ativo" && operations.includes("deactivate") && !parentHibernated && <details className="resource-danger"><summary>Hibernar</summary><form action={manageResourceAction}><input type="hidden" name="integrationId" value={integrationId} /><input type="hidden" name="resourceType" value={resourceType} /><input type="hidden" name="resourceId" value={item.id} /><input type="hidden" name="accountId" value={resourceType === "account" ? "" : String(item.accountId ?? params.accountId ?? "")} /><input type="hidden" name="operation" value="resource.deactivate" /><input type="hidden" name="idempotencyKey" value={randomUUID()} /><p>O registro fica indisponível e pode ser reativado durante o período de retenção.</p><button type="submit">Confirmar hibernação</button></form></details>}
              {state === "Hibernado" && operations.includes("purge") && !parentHibernated && <details className="resource-danger"><summary>Purga definitiva</summary><form action={manageResourceAction}><input type="hidden" name="integrationId" value={integrationId} /><input type="hidden" name="resourceType" value={resourceType} /><input type="hidden" name="resourceId" value={item.id} /><input type="hidden" name="accountId" value={resourceType === "account" ? "" : String(item.accountId ?? params.accountId ?? "")} /><input type="hidden" name="operation" value="resource.purge" /><input type="hidden" name="idempotencyKey" value={randomUUID()} /><p>{purgeDue ? `A purga ${purge?.mode === "anonymize" ? "anonimiza" : "apaga"} este registro${purge?.cascades?.length ? ` e os dependentes: ${purge.cascades.join(", ")}` : ""}. Esta ação não pode ser desfeita.` : `Disponível após ${purgeDate ? purgeDate.toLocaleDateString("pt-BR") : "o fim do período de retenção"}.`}</p>{purgeDue && <><label>Digite PURGAR para confirmar<input name="confirmation" autoComplete="off" required /></label><button type="submit">Apagar definitivamente</button></>}</form></details>}
            </div>
          </article>;
        })}
        {nextCursor && <nav className="resource-pagination" aria-label="Paginação"><span>Mostrando até {items.length} registros</span><a className="quiet-button" href={`/accounts?${new URLSearchParams({ integrationId, resourceType, lifecycle, ...(params.accountId ? { accountId: params.accountId } : {}), ...(params.search ? { search: params.search } : {}), cursor: nextCursor }).toString()}`}>Próxima página →</a></nav>}
      </section>}
    </>}
  </div>;
}
