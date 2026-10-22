"use server";

import { redirect } from "next/navigation";
import { z } from "zod";
import { db } from "@/lib/server/db";
import { decryptSecret, encryptSecret } from "@/lib/server/crypto";
import { discoverCapabilities, executeProvisioning, executeOperation, type BootstrapPayload, type ContractCapabilities } from "@/lib/server/provisioning";

const BootstrapFormSchema = z.object({
  integrationId: z.string().min(1),
  contractVersion: z.literal("1"),
  operation: z.literal("bootstrap"),
  input: z.object({
    account: z.object({ name: z.string().trim().min(1).max(160), taxId: z.string().trim().min(11).max(18), email: z.string().trim().email().max(254), address: z.string().trim().min(1).max(240) }),
    initialCustomer: z.object({ tradeName: z.string().trim().min(1).max(160), legalName: z.string().trim().max(160), taxId: z.string().trim().min(11).max(18), email: z.union([z.literal(""), z.string().trim().email().max(254)]), phone: z.string().max(20), taxRegime: z.string().trim().min(1).max(120), relationshipType: z.enum(["Recorrente", "Avulso"]), status: z.enum(["Ativo", "Inativo"]) }),
    initialUser: z.object({ name: z.string().trim().min(1).max(160), email: z.string().trim().email().max(254), phone: z.string().max(20) }),
  }),
});
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
  const encrypted = encryptSecret(apiKey);
  let probe: Response;
  try {
    probe = await fetch(`${baseUrl}/api/v1/provisioning`, { headers: { "X-Lynx-API-Key": apiKey }, cache: "no-store", signal: AbortSignal.timeout(12000) });
  } catch (error) {
    if (error && typeof error === "object" && "digest" in error) throw error;
    redirect("/integrations?erro=conexao");
  }
  const capabilities = await probe.json().catch(() => null) as { contractVersion?: string } | null;
  if (!probe.ok || capabilities?.contractVersion !== "1") redirect("/integrations?erro=conexao");
  try {
    await db.integration.create({ data: { name, slug, baseUrl, appUrl: appUrl || null, encryptedApiKey: encrypted.ciphertext, apiKeyIv: encrypted.iv, apiKeyAuthTag: encrypted.authTag } });
  } catch {
    redirect("/integrations?erro=duplicada");
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
  const existing = await db.integration.findUnique({ where: { id } });
  if (!existing || existing.revokedAt) redirect("/integrations?erro=nao-encontrada");
  try {
    const apiKey = decryptSecret(existing.encryptedApiKey, existing.apiKeyIv, existing.apiKeyAuthTag);
    const response = await fetch(`${baseUrl}/api/v1/provisioning`, { headers: { "X-Lynx-API-Key": apiKey }, cache: "no-store", signal: AbortSignal.timeout(12000) });
    const capabilities = await response.json().catch(() => null) as { contractVersion?: string } | null;
    if (!response.ok || capabilities?.contractVersion !== "1") redirect("/integrations?erro=conexao");
  } catch (error) {
    if (error && typeof error === "object" && "digest" in error) throw error;
    redirect("/integrations?erro=conexao");
  }
  const slug = name.toLocaleLowerCase("pt-BR").normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  try {
    await db.integration.update({ where: { id }, data: { name, slug, baseUrl, appUrl: appUrl || null } });
  } catch {
    redirect("/integrations?erro=duplicada");
  }
  await db.auditLog.create({ data: { action: "integration.update-metadata", integrationId: id, result: "COMPLETED" } });
  redirect("/integrations?atualizado=1");
}

export async function toggleIntegrationAction(formData: FormData) {
  await requireRoles(["owner", "admin"]);
  const id = String(formData.get("id") ?? "");
  const integration = await db.integration.findUnique({ where: { id }, select: { active: true, revokedAt: true } });
  if (!integration) redirect("/integrations?erro=nao-encontrada");
  if (integration.revokedAt) redirect("/integrations?erro=revogada");
  await db.integration.update({ where: { id }, data: { active: !integration.active } });
  await db.auditLog.create({ data: { action: integration.active ? "integration.pause" : "integration.activate", integrationId: id, result: "COMPLETED" } });
  redirect("/integrations?atualizado=1");
}

export async function revokeIntegrationAction(formData: FormData) {
  await requireRoles(["owner", "admin"]);
  const id = String(formData.get("id") ?? "");
  const integration = await db.integration.findUnique({ where: { id }, select: { revokedAt: true } });
  if (!integration) redirect("/integrations?erro=nao-encontrada");
  if (integration.revokedAt) redirect("/integrations?revogada=1");
  await db.$transaction([
    db.integration.update({ where: { id }, data: { active: false, encryptedApiKey: "", apiKeyIv: "", apiKeyAuthTag: "", revokedAt: new Date() } }),
    db.auditLog.create({ data: { action: "integration.revoke-local-key", integrationId: id, result: "COMPLETED" } }),
  ]);
  redirect("/integrations?revogada=1");
}

export async function rotateIntegrationKeyAction(formData: FormData) {
  await requireRoles(["owner", "admin"]);
  const id = String(formData.get("id") ?? "");
  const apiKey = String(formData.get("apiKey") ?? "").trim();
  const integration = await db.integration.findUnique({ where: { id } });
  if (!integration || integration.revokedAt || !apiKey) redirect("/integrations?erro=rotacao");
  try {
    const response = await fetch(`${integration.baseUrl}/api/v1/provisioning`, { headers: { "X-Lynx-API-Key": apiKey }, cache: "no-store", signal: AbortSignal.timeout(12000) });
    const capabilities = await response.json().catch(() => null) as { contractVersion?: string } | null;
    if (!response.ok || capabilities?.contractVersion !== "1") redirect("/integrations?erro=chave");
  } catch (error) {
    if (error && typeof error === "object" && "digest" in error) throw error;
    redirect("/integrations?erro=chave");
  }
  const encrypted = encryptSecret(apiKey);
  await db.integration.update({ where: { id }, data: { encryptedApiKey: encrypted.ciphertext, apiKeyIv: encrypted.iv, apiKeyAuthTag: encrypted.authTag } });
  await db.auditLog.create({ data: { action: "integration.rotate-key", integrationId: id, result: "COMPLETED" } });
  redirect("/integrations?rotacionada=1");
}

export async function provisionAction(formData: FormData) {
  await requireRoles(["owner", "admin", "operator"]);
  const integrationId = String(formData.get("integrationId") ?? "");
  const text = (name: string) => String(formData.get(name) ?? "").trim();
  const payloadCandidate = {
    contractVersion: "1",
    operation: "bootstrap",
    input: {
      account: { name: text("accountName"), taxId: text("accountTaxId"), email: text("accountEmail"), address: text("accountAddress") },
      initialCustomer: { tradeName: text("customerTradeName"), legalName: text("customerLegalName"), taxId: text("customerTaxId"), email: text("customerEmail"), phone: text("customerPhone"), taxRegime: text("customerTaxRegime"), relationshipType: text("relationshipType"), status: text("customerStatus") },
      initialUser: { name: text("userName"), email: text("userEmail"), phone: text("userPhone") },
    },
  };
  const parsedPayload = BootstrapFormSchema.safeParse({ integrationId, ...payloadCandidate });
  if (!parsedPayload.success) {
    redirect("/provisioning?erro=campos");
  }
  const payload = parsedPayload.data as BootstrapPayload;
  const idempotencyKey = text("idempotencyKey");
  if (!idempotencyKey) redirect("/provisioning?erro=idempotencia");
  try {
    await executeProvisioning(integrationId, payload, idempotencyKey);
    redirect(`/provisioning?request=${encodeURIComponent(idempotencyKey)}`);
  } catch (error) {
    if (error && typeof error === "object" && "digest" in error) throw error;
    redirect(`/provisioning?erro=conexao&request=${encodeURIComponent(idempotencyKey)}`);
  }
}

export async function retryProvisionAction(formData: FormData) {
  await requireRoles(["owner", "admin", "operator"]);
  const idempotencyKey = String(formData.get("idempotencyKey") ?? "");
  const record = await db.provisioningRecord.findUnique({ where: { idempotencyKey } });
  if (!record || record.status !== "RETRYABLE") redirect("/provisioning?erro=solicitacao");
  try {
    const payload = decryptSecret(record.payloadCiphertext, record.payloadIv, record.payloadAuthTag);
    await executeProvisioning(record.integrationId, JSON.parse(payload) as BootstrapPayload, idempotencyKey);
    redirect(`/provisioning?request=${encodeURIComponent(idempotencyKey)}`);
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
  const record = await db.provisioningRecord.findUnique({ where: { idempotencyKey } });
  if (!record || record.status !== "RETRYABLE") redirect("/accounts?erro=solicitacao");
  try {
    const payload = JSON.parse(decryptSecret(record.payloadCiphertext, record.payloadIv, record.payloadAuthTag)) as { operation?: string; input?: Record<string, unknown> };
    if (!payload.operation || !payload.input) redirect("/accounts?erro=solicitacao");
    await executeOperation(record.integrationId, payload.operation, payload.input, idempotencyKey);
    const resourceType = String(payload.input.resourceType ?? "account");
    const accountId = String(payload.input.accountId ?? "");
    redirect(resourceLocation({ integrationId: record.integrationId, resourceType, accountId, request: idempotencyKey }));
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
