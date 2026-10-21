"use server";

import { redirect } from "next/navigation";
import { z } from "zod";
import { db } from "@/lib/server/db";
import { decryptSecret, encryptSecret } from "@/lib/server/crypto";
import { executeProvisioning, type BootstrapPayload } from "@/lib/server/provisioning";

const BootstrapFormSchema = z.object({
  integrationId: z.string().min(1),
  account: z.object({ name: z.string().trim().min(1).max(160), taxId: z.string().trim().min(11).max(18), email: z.string().trim().email().max(254), address: z.string().trim().min(1).max(240) }),
  initialCustomer: z.object({ tradeName: z.string().trim().min(1).max(160), legalName: z.string().trim().max(160), taxId: z.string().trim().min(11).max(18), email: z.union([z.literal(""), z.string().trim().email().max(254)]), phone: z.string().max(20), taxRegime: z.string().trim().min(1).max(120), relationshipType: z.enum(["Recorrente", "Avulso"]), status: z.enum(["Ativo", "Inativo"]) }),
  initialUser: z.object({ name: z.string().trim().min(1).max(160), email: z.string().trim().email().max(254), phone: z.string().max(20) }),
});
import { requireSession, signIn, signOut } from "@/lib/server/session";

export async function loginAction(formData: FormData) {
  const email = String(formData.get("email") ?? "");
  const password = String(formData.get("password") ?? "");
  const ok = await signIn(email, password);
  if (ok) redirect("/provisioning");
  redirect("/login?erro=credenciais");
}

export async function logoutAction() {
  await signOut();
  redirect("/login");
}

export async function addIntegrationAction(formData: FormData) {
  await requireSession();
  const name = String(formData.get("name") ?? "").trim();
  const baseUrl = String(formData.get("baseUrl") ?? "").trim().replace(/\/$/, "");
  const apiKey = String(formData.get("apiKey") ?? "").trim();
  if (!name || !apiKey) redirect("/integrations?erro=campos");
  let parsedUrl: URL;
  try { parsedUrl = new URL(baseUrl); } catch { redirect("/integrations?erro=url"); }
  if (parsedUrl.protocol !== "https:" && parsedUrl.hostname !== "localhost") redirect("/integrations?erro=https");
  if (parsedUrl.username || parsedUrl.password || parsedUrl.search || parsedUrl.hash) redirect("/integrations?erro=url");

  const slug = name.toLocaleLowerCase("pt-BR").normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  const encrypted = encryptSecret(apiKey);
  try {
    await db.integration.create({ data: { name, slug, baseUrl, encryptedApiKey: encrypted.ciphertext, apiKeyIv: encrypted.iv, apiKeyAuthTag: encrypted.authTag } });
  } catch {
    redirect("/integrations?erro=duplicada");
  }
  redirect("/integrations?salvo=1");
}

export async function toggleIntegrationAction(formData: FormData) {
  await requireSession();
  const id = String(formData.get("id") ?? "");
  const integration = await db.integration.findUnique({ where: { id }, select: { active: true, revokedAt: true } });
  if (!integration) redirect("/integrations?erro=nao-encontrada");
  if (integration.revokedAt) redirect("/integrations?erro=revogada");
  await db.integration.update({ where: { id }, data: { active: !integration.active } });
  await db.auditLog.create({ data: { action: integration.active ? "integration.pause" : "integration.activate", integrationId: id, result: "COMPLETED" } });
  redirect("/integrations?atualizado=1");
}

export async function revokeIntegrationAction(formData: FormData) {
  await requireSession();
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
  await requireSession();
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
  await requireSession();
  const integrationId = String(formData.get("integrationId") ?? "");
  const text = (name: string) => String(formData.get(name) ?? "").trim();
  const payloadCandidate = {
    contractVersion: "1",
    operation: "bootstrap",
    account: { name: text("accountName"), taxId: text("accountTaxId"), email: text("accountEmail"), address: text("accountAddress") },
    initialCustomer: { tradeName: text("customerTradeName"), legalName: text("customerLegalName"), taxId: text("customerTaxId"), email: text("customerEmail"), phone: text("customerPhone"), taxRegime: text("customerTaxRegime"), relationshipType: text("relationshipType"), status: text("customerStatus") },
    initialUser: { name: text("userName"), email: text("userEmail"), phone: text("userPhone"), role: "owner", invite: true },
  };
  const parsedPayload = BootstrapFormSchema.safeParse({ integrationId, ...payloadCandidate });
  if (!parsedPayload.success) {
    redirect("/provisioning?erro=campos");
  }
  const payload = { contractVersion: "1", operation: "bootstrap", account: parsedPayload.data.account, initialCustomer: parsedPayload.data.initialCustomer, initialUser: { ...parsedPayload.data.initialUser, role: "owner", invite: true } } as BootstrapPayload;
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
  await requireSession();
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
