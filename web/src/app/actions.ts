"use server";

import { redirect } from "next/navigation";
import { randomUUID } from "node:crypto";
import { z } from "zod";
import { lynxApi, LynxApiError } from "@/lib/server/lynx-api";
import { discoverCapabilities, executeProvisioning, executeOperation, listIntegrations, type BootstrapPayload, type ContractCapabilities } from "@/lib/server/provisioning";

const requiredText = (label: string, max: number) => z.string().trim().min(1, `Preencha ${label}.`).max(max, `${label} deve ter no máximo ${max} caracteres.`);
const taxId = (account = false) => z.string().trim().refine((value) => {
  const digits = value.replace(/\D/g, "");
  return account ? digits.length === 14 : digits.length === 11 || digits.length === 14;
}, account ? "Informe um CNPJ com 14 dígitos." : "Informe um CPF com 11 dígitos ou CNPJ com 14 dígitos.");

const BootstrapFormSchema = z.object({
  integrationId: z.string().min(1, "Selecione o SaaS."),
  contractVersion: z.literal("1"),
  operation: z.literal("bootstrap"),
  input: z.object({
    account: z.object({ name: requiredText("o nome do escritório", 160), taxId: taxId(true), email: z.string().trim().email("Informe um e-mail válido para o escritório.").max(254), address: requiredText("o endereço completo", 240) }),
    initialCustomer: z.object({ tradeName: requiredText("o nome fantasia", 160), legalName: z.string().trim().max(160, "A razão social deve ter no máximo 160 caracteres."), taxId: taxId(), email: z.union([z.literal(""), z.string().trim().email("Informe um e-mail válido para o cliente.").max(254)]), phone: z.string().max(20, "O telefone do cliente deve ter no máximo 20 caracteres."), taxRegime: requiredText("o regime tributário", 120), relationshipType: z.enum(["Recorrente", "Avulso"], { error: "Selecione o tipo de relação." }), status: z.enum(["Ativo", "Inativo"], { error: "Selecione o status do cliente." }) }),
    initialUser: z.object({ name: requiredText("o nome da pessoa responsável", 160), email: z.string().trim().email("Informe um e-mail válido para o acesso." ).max(254), phone: z.string().max(20, "O telefone deve ter no máximo 20 caracteres.") }),
  }),
});

export type ProvisionFormState = {
  values: Record<string, string>;
  fieldErrors: Record<string, string>;
  message?: string;
  revision: number;
};

const provisionFieldNames = ["integrationId", "accountName", "accountTaxId", "accountEmail", "accountAddress", "customerTradeName", "customerLegalName", "customerTaxId", "customerEmail", "customerPhone", "customerTaxRegime", "relationshipType", "customerStatus", "userName", "userEmail", "userPhone", "idempotencyKey"];
const issueFieldMap: Record<string, string> = {
  integrationId: "integrationId",
  "input.account.name": "accountName", "input.account.taxId": "accountTaxId", "input.account.email": "accountEmail", "input.account.address": "accountAddress",
  "input.initialCustomer.tradeName": "customerTradeName", "input.initialCustomer.legalName": "customerLegalName", "input.initialCustomer.taxId": "customerTaxId", "input.initialCustomer.email": "customerEmail", "input.initialCustomer.phone": "customerPhone", "input.initialCustomer.taxRegime": "customerTaxRegime", "input.initialCustomer.relationshipType": "relationshipType", "input.initialCustomer.status": "customerStatus",
  "input.initialUser.name": "userName", "input.initialUser.email": "userEmail", "input.initialUser.phone": "userPhone",
};

import { requireRoles } from "@/lib/server/session";

export async function addIntegrationAction(formData: FormData) {
  await requireRoles(["owner", "admin"]);
  const name = String(formData.get("name") ?? "").trim();
  const baseUrl = String(formData.get("baseUrl") ?? "").trim().replace(/\/$/, "");
  const appUrl = String(formData.get("appUrl") ?? "").trim().replace(/\/$/, "");
  const apiKey = String(formData.get("apiKey") ?? "").trim();
  if (!name || !apiKey) redirect("/integrations?erro=campos");
  let parsedUrl: URL;
  try { parsedUrl = new URL(baseUrl); } catch { redirect("/integrations?erro=url"); }
  if (parsedUrl.protocol !== "https:" && parsedUrl.hostname !== "localhost") redirect("/integrations?erro=https");
  if (parsedUrl.username || parsedUrl.password || parsedUrl.search || parsedUrl.hash) redirect("/integrations?erro=url");
  let parsedAppUrl: URL | null = null;
  if (appUrl) {
    try { parsedAppUrl = new URL(appUrl); } catch { redirect("/integrations?erro=app-url"); }
    if (parsedAppUrl.protocol !== "https:" && parsedAppUrl.hostname !== "localhost") redirect("/integrations?erro=app-url");
    if (parsedAppUrl.username || parsedAppUrl.password || parsedAppUrl.search || parsedAppUrl.hash) redirect("/integrations?erro=app-url");
  }

  const slug = name.toLocaleLowerCase("pt-BR").normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  try {
    await lynxApi("/integrations", { method: "POST", body: JSON.stringify({ name, slug, baseUrl, appUrl, apiKey }) });
  } catch {
    redirect("/integrations?erro=conexao");
  }
  redirect("/integrations?salvo=1");
}

export async function editIntegrationAction(formData: FormData) {
  await requireRoles(["owner", "admin"]);
  const id = String(formData.get("id") ?? "");
  const name = String(formData.get("name") ?? "").trim();
  const baseUrl = String(formData.get("baseUrl") ?? "").trim().replace(/\/$/, "");
  const appUrl = String(formData.get("appUrl") ?? "").trim().replace(/\/$/, "");
  if (!id || !name || name.length > 120) redirect("/integrations?erro=campos");
  let apiUrl: URL;
  try { apiUrl = new URL(baseUrl); } catch { redirect("/integrations?erro=url"); }
  if ((apiUrl.protocol !== "https:" && apiUrl.hostname !== "localhost") || apiUrl.username || apiUrl.password || apiUrl.search || apiUrl.hash) redirect("/integrations?erro=url");
  if (appUrl) {
    let app: URL;
    try { app = new URL(appUrl); } catch { redirect("/integrations?erro=app-url"); }
    if ((app.protocol !== "https:" && app.hostname !== "localhost") || app.username || app.password || app.search || app.hash) redirect("/integrations?erro=app-url");
  }
  try {
    const slug = name.toLocaleLowerCase("pt-BR").normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
    await lynxApi(`/integrations/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify({ name, slug, baseUrl, appUrl }) });
  } catch {
    redirect("/integrations?erro=conexao");
  }
  redirect("/integrations?atualizado=1");
}

export async function toggleIntegrationAction(formData: FormData) {
  await requireRoles(["owner", "admin"]);
  const id = String(formData.get("id") ?? "");
  const integration = (await listIntegrations()).find((item) => item.id === id);
  if (!integration) redirect("/integrations?erro=nao-encontrada");
  try { await lynxApi(`/integrations/${encodeURIComponent(id)}/active`, { method: "PATCH", body: JSON.stringify({ active: !integration.active }) }); }
  catch { redirect("/integrations?erro=revogada"); }
  redirect("/integrations?atualizado=1");
}

export async function revokeIntegrationAction(formData: FormData) {
  await requireRoles(["owner", "admin"]);
  const id = String(formData.get("id") ?? "");
  try { await lynxApi(`/integrations/${encodeURIComponent(id)}`, { method: "DELETE" }); }
  catch { redirect("/integrations?erro=nao-encontrada"); }
  redirect("/integrations?revogada=1");
}

export async function rotateIntegrationKeyAction(formData: FormData) {
  await requireRoles(["owner", "admin"]);
  const id = String(formData.get("id") ?? "");
  const apiKey = String(formData.get("apiKey") ?? "").trim();
  if (!id || !apiKey) redirect("/integrations?erro=rotacao");
  try {
    await lynxApi(`/integrations/${encodeURIComponent(id)}/api-key`, { method: "PUT", body: JSON.stringify({ apiKey }) });
  } catch {
    redirect("/integrations?erro=chave");
  }
  redirect("/integrations?rotacionada=1");
}

export async function provisionAction(_previousState: ProvisionFormState, formData: FormData): Promise<ProvisionFormState> {
  await requireRoles(["owner", "admin", "operator"]);
  const values = Object.fromEntries(provisionFieldNames.map((name) => [name, String(formData.get(name) ?? "").trim()]));
  const integrationId = values.integrationId;
  const text = (name: string) => values[name] ?? "";
  const payloadCandidate = {
    contractVersion: "1",
    operation: "bootstrap",
    input: {
      account: { name: text("accountName"), taxId: text("accountTaxId").replace(/\D/g, ""), email: text("accountEmail"), address: text("accountAddress") },
      initialCustomer: { tradeName: text("customerTradeName"), legalName: text("customerLegalName"), taxId: text("customerTaxId").replace(/\D/g, ""), email: text("customerEmail"), phone: text("customerPhone"), taxRegime: text("customerTaxRegime"), relationshipType: text("relationshipType"), status: text("customerStatus") },
      initialUser: { name: text("userName"), email: text("userEmail"), phone: text("userPhone") },
    },
  };
  const parsedPayload = BootstrapFormSchema.safeParse({ integrationId, ...payloadCandidate });
  if (!parsedPayload.success) {
    const fieldErrors: Record<string, string> = {};
    for (const issue of parsedPayload.error.issues) {
      const fieldName = issueFieldMap[issue.path.join(".")];
      if (fieldName && !fieldErrors[fieldName]) fieldErrors[fieldName] = issue.message;
    }
    return { values, fieldErrors, message: "Não foi enviado. Corrija os campos destacados; os dados preenchidos foram mantidos.", revision: _previousState.revision + 1 };
  }
  const payload = parsedPayload.data as BootstrapPayload;
  const idempotencyKey = text("idempotencyKey");
  if (!idempotencyKey) return { values, fieldErrors: { idempotencyKey: "Atualize a página e tente novamente." }, message: "Não foi possível identificar esta solicitação.", revision: _previousState.revision + 1 };
  try {
    await executeProvisioning(integrationId, payload, idempotencyKey);
    redirect(`/provisioning?integrationId=${encodeURIComponent(integrationId)}&request=${encodeURIComponent(idempotencyKey)}`);
  } catch (error) {
    if (error && typeof error === "object" && "digest" in error) throw error;
    const message = error instanceof LynxApiError
      ? error.message
      : "Não foi possível confirmar a resposta do sistema. Seus dados foram mantidos; tente novamente antes de fechar esta página.";
    const fieldErrors: Record<string, string> = error instanceof LynxApiError && error.code === "saas_validation_failed"
      ? { _form: message }
      : {};
    return { values: { ...values, idempotencyKey: error instanceof LynxApiError && error.code === "saas_validation_failed" ? randomUUID() : idempotencyKey }, fieldErrors, message, revision: _previousState.revision + 1 };
  }
}

export async function retryProvisionAction(formData: FormData) {
  await requireRoles(["owner", "admin", "operator"]);
  const idempotencyKey = String(formData.get("idempotencyKey") ?? "");
  const integrationId = String(formData.get("integrationId") ?? "");
  if (!integrationId || !idempotencyKey) redirect("/provisioning?erro=solicitacao");
  try {
    await lynxApi(`/integrations/${encodeURIComponent(integrationId)}/operations/${encodeURIComponent(idempotencyKey)}/retry`, { method: "POST", body: "{}" });
    redirect(`/provisioning?integrationId=${encodeURIComponent(integrationId)}&request=${encodeURIComponent(idempotencyKey)}`);
  } catch (error) {
    if (error && typeof error === "object" && "digest" in error) throw error;
    redirect(`/provisioning?erro=conexao&request=${encodeURIComponent(idempotencyKey)}`);
  }
}

export async function manageResourceAction(formData: FormData) {
  await requireRoles(["owner", "admin", "operator"]);
  const integrationId = String(formData.get("integrationId") ?? "");
  const resourceType = String(formData.get("resourceType") ?? "");
  const accountId = String(formData.get("accountId") ?? "");
  const resourceId = String(formData.get("resourceId") ?? "");
  const operation = String(formData.get("operation") ?? "");
  const idempotencyKey = String(formData.get("idempotencyKey") ?? "");
  if (!integrationId || !["account", "customer", "user"].includes(resourceType) || !["resource.update", "resource.deactivate", "resource.reactivate", "resource.purge"].includes(operation) || !/^[a-zA-Z0-9._:-]{8,128}$/.test(idempotencyKey)) {
    redirect("/accounts?erro=solicitacao");
  }
  if (operation === "resource.purge" && String(formData.get("confirmation") ?? "").trim() !== "PURGAR") {
    redirect(resourceLocation({ integrationId, resourceType, accountId, erro: "confirmacao" }));
  }
  const input: Record<string, unknown> = { resourceType, resourceId };
  if (resourceType !== "account") input.accountId = accountId;
  if (operation === "resource.update") {
    let capabilities: ContractCapabilities;
    try { capabilities = await discoverCapabilities(integrationId); }
    catch { redirect(resourceLocation({ integrationId, resourceType, accountId, erro: "integracao" })); }
    const editable = capabilities.resources[resourceType]?.fields?.editable;
    if (!Array.isArray(editable)) redirect(resourceLocation({ integrationId, resourceType, accountId, erro: "campos" }));
    const changes: Record<string, string> = {};
    for (const [key, value] of formData.entries()) {
      if (!key.startsWith("change.")) continue;
      const field = key.slice("change.".length);
      if (!editable.includes(field) || typeof value !== "string" || value.length > 240) {
        redirect(resourceLocation({ integrationId, resourceType, accountId, erro: "campos" }));
      }
      changes[field] = value.trim();
    }
    if (Object.keys(changes).length === 0) redirect(resourceLocation({ integrationId, resourceType, accountId, erro: "campos" }));
    input.changes = changes;
  }
  try {
    const { status } = await executeOperation(integrationId, operation, input, idempotencyKey);
    redirect(resourceLocation({ integrationId, resourceType, accountId, request: idempotencyKey, result: status }));
  } catch (error) {
    if (error && typeof error === "object" && "digest" in error) throw error;
    redirect(resourceLocation({ integrationId, resourceType, accountId, erro: "integracao" }));
  }
}

export async function retryResourceAction(formData: FormData) {
  await requireRoles(["owner", "admin", "operator"]);
  const idempotencyKey = String(formData.get("idempotencyKey") ?? "");
  const integrationId = String(formData.get("integrationId") ?? "");
  if (!integrationId || !idempotencyKey) redirect("/accounts?erro=solicitacao");
  try {
    await lynxApi(`/integrations/${encodeURIComponent(integrationId)}/operations/${encodeURIComponent(idempotencyKey)}/retry`, { method: "POST", body: "{}" });
    const resourceType = String(formData.get("resourceType") ?? "account");
    const accountId = String(formData.get("accountId") ?? "");
    redirect(resourceLocation({ integrationId, resourceType, accountId, request: idempotencyKey }));
  } catch (error) {
    if (error && typeof error === "object" && "digest" in error) throw error;
    redirect("/accounts?erro=integracao");
  }
}

function resourceLocation(params: Record<string, string>) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) if (value) query.set(key, value);
  return `/accounts?${query.toString()}`;
}
