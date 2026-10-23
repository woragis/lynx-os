import Link from "next/link";
import { addIntegrationAction } from "@/app/actions";
import { IntegrationCatalog } from "@/components/integration-catalog";
import { listIntegrations } from "@/lib/server/provisioning";
import { requireSession } from "@/lib/server/session";

export default async function IntegrationsPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  const params = await searchParams;
  await requireSession();
  const integrations = await listIntegrations();
  const errorMessages: Record<string, string> = {
    conexao: "Não conseguimos validar o contrato do SaaS. Confira o endereço da API e a API key.",
    "app-url": "Informe uma URL HTTPS válida para abrir o painel do SaaS.",
    chave: "A nova chave não tem acesso ao contrato do SaaS ou a conexão falhou.",
    rotacao: "Informe uma chave válida para uma integração ativa.",
    "nao-encontrada": "A integração não existe ou já foi revogada.",
    revogada: "Uma chave revogada não pode ser ativada novamente; cadastre uma nova integração.",
    https: "Use uma URL HTTPS válida, sem usuário, senha, query ou fragmento.",
    url: "Use uma URL HTTPS válida, sem usuário, senha, query ou fragmento.",
    duplicada: "Já existe um SaaS com esse nome. Escolha outro nome.",
  };

  return <div className="page-wrap integrations-page">
    <div className="page-heading"><div><p className="eyebrow">ECOSSISTEMA LYNX</p><h1>Integrações</h1><p className="muted">Conecte e opere seus SaaS em um único lugar. Cada integração permite cadastrar e consultar empresas, clientes e usuários.</p></div><span className="count-badge">{integrations.length.toString().padStart(2, "0")} sistemas</span></div>
    {params.erro && <p className="error-banner" role="alert">{errorMessages[params.erro] ?? "Confira os campos obrigatórios e tente novamente."}</p>}
    {params.salvo && <p className="success-banner">Integração salva. A chave não será exibida novamente.</p>}
    {params.atualizado && <p className="success-banner">Dados da integração atualizados.</p>}
    {params.rotacionada && <p className="success-banner">Chave validada e substituída no Lynx OS. Revogue a chave antiga no painel do SaaS.</p>}
    {params.revogada && <p className="success-banner">Chave apagada do Lynx OS e integração bloqueada. Revogue-a também no SaaS para invalidá-la remotamente.</p>}

    <section className="panel integration-connect-panel">
      <div className="integration-connect-copy"><span className="panel-icon">＋</span><div><h2>Conectar um SaaS</h2><p>Registre o domínio da API e uma chave de provisionamento. A chave será cifrada e nunca aparecerá novamente.</p></div></div>
      <form action={addIntegrationAction} className="integration-connect-form">
        <label>Nome do sistema<input name="name" placeholder="EasyBill" required maxLength={120} /></label>
        <label>URL da API<input name="baseUrl" type="url" placeholder="https://api.seusistema.com.br" required /></label>
        <label>URL do painel <span className="optional">(opcional)</span><input name="appUrl" type="url" placeholder="https://seusistema.com.br/login" /></label>
        <label className="integration-api-key">API key<input name="apiKey" type="password" autoComplete="new-password" placeholder="Chave com provisioning:read e provisioning:write" required /></label>
        <button className="button button-dark" type="submit">Validar e conectar <span aria-hidden>↗</span></button>
      </form>
      <p className="integration-connect-footnote">A conexão é validada com o contrato Lynx v1. A chave é protegida com AES-256-GCM.</p>
    </section>

    <section className="integration-catalog-section">
      <div className="integration-section-heading"><div><p className="eyebrow">SISTEMAS CONECTADOS</p><h2>Seus SaaS</h2><p>Abra o painel, consulte cadastros ou comece um novo cadastro já neste sistema.</p></div><Link href="/accounts" className="quiet-button">Ver todos os cadastros ↗</Link></div>
      {integrations.length === 0
        ? <div className="empty-state integration-empty"><span className="empty-mark">◈</span><h2>Nenhum SaaS conectado ainda</h2><p>Conecte seu primeiro sistema acima para cadastrar empresas, clientes e usuários pelo Lynx OS.</p></div>
        : <IntegrationCatalog integrations={integrations} />}
    </section>

    <div className="security-note"><span>⌑</span><p><strong>Credenciais protegidas.</strong> Revogar apaga a chave daqui e bloqueia chamadas; revogue-a também no painel do SaaS para invalidá-la remotamente. Mantenha <code>LYNX_ENCRYPTION_KEY</code> fora do banco e do repositório.</p></div>
  </div>;
}
