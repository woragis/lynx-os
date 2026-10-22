import "server-only";

import { createHash } from "node:crypto";
import { db } from "@/lib/server/db";
import { decryptSecret, encryptSecret } from "@/lib/server/crypto";

export type BootstrapPayload = {
  contractVersion: "1";
  operation: "bootstrap";
  input: {
    account: { name: string; taxId: string; email: string; address: string };
    initialCustomer: { tradeName: string; legalName: string; taxId: string; email: string; phone: string; taxRegime: string; relationshipType: string; status: string };
    initialUser: { name: string; email: string; phone: string };
  };
};

export async function executeProvisioning(integrationId: string, payload: BootstrapPayload, idempotencyKey: string) {
  const integration = await db.integration.findUnique({ where: { id: integrationId } });
  if (!integration || !integration.active) throw new Error("Integração não encontrada ou pausada.");
  const payloadHash = createHash("sha256").update(JSON.stringify(payload)).digest("hex");
  let record = await db.provisioningRecord.findUnique({ where: { idempotencyKey } });
  if (record && (record.integrationId !== integrationId || record.payloadHash !== payloadHash)) throw new Error("Idempotency-Key já foi usada com outro pedido.");
  if (record?.responseCiphertext && record.responseIv && record.responseAuthTag) {
    const response = decryptSecret(record.responseCiphertext, record.responseIv, record.responseAuthTag);
    return { status: record.status, body: JSON.parse(response) as unknown };
  }
  if (!record) {
    try {
      const encryptedPayload = encryptSecret(JSON.stringify(payload));
      record = await db.provisioningRecord.create({ data: { idempotencyKey, integrationId, payloadHash, payloadCiphertext: encryptedPayload.ciphertext, payloadIv: encryptedPayload.iv, payloadAuthTag: encryptedPayload.authTag, status: "PENDING" } });
    } catch {
      record = await db.provisioningRecord.findUnique({ where: { idempotencyKey } });
      if (!record || record.integrationId !== integrationId || record.payloadHash !== payloadHash) throw new Error("Não foi possível reservar esta operação idempotente.");
      if (record.responseCiphertext && record.responseIv && record.responseAuthTag) {
        const response = decryptSecret(record.responseCiphertext, record.responseIv, record.responseAuthTag);
        return { status: record.status, body: JSON.parse(response) as unknown };
      }
    }
  }

  const apiKey = decryptSecret(integration.encryptedApiKey, integration.apiKeyIv, integration.apiKeyAuthTag);
  try {
    const apiUrl = `${integration.baseUrl}/api/v1/provisioning`;
    const capabilityResponse = await fetch(apiUrl, { headers: { "X-Lynx-API-Key": apiKey }, cache: "no-store", signal: AbortSignal.timeout(12000) });
    if (!capabilityResponse.ok) throw new Error(`Falha ao consultar capacidades do SaaS (${capabilityResponse.status}).`);
    const capabilities = await capabilityResponse.json() as { contractVersion?: string };
    if (capabilities.contractVersion !== "1") throw new Error("Versão do contrato incompatível.");
    const response = await fetch(apiUrl, { method: "POST", headers: { "X-Lynx-API-Key": apiKey, "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(payload), cache: "no-store", signal: AbortSignal.timeout(30000) });
    const body: unknown = await response.json().catch(() => ({}));
    const serialized = JSON.stringify(body);
    const retryable = response.status === 429 || response.status >= 500;
    const status = retryable ? "RETRYABLE" : response.ok ? "COMPLETED" : `HTTP_${response.status}`;
    const encryptedResponse = retryable ? null : encryptSecret(serialized);
    await db.provisioningRecord.update({ where: { idempotencyKey }, data: {
      status,
      responseCiphertext: encryptedResponse?.ciphertext ?? null,
      responseIv: encryptedResponse?.iv ?? null,
      responseAuthTag: encryptedResponse?.authTag ?? null,
    } });
    await db.auditLog.create({ data: { action: "provision.bootstrap", integrationId, requestId: idempotencyKey, result: retryable ? `${status}_HTTP_${response.status}` : status } });
    return { status, body };
  } catch (error) {
    await db.provisioningRecord.update({ where: { idempotencyKey }, data: { status: "RETRYABLE" } });
    await db.auditLog.create({ data: { action: "provision.bootstrap", integrationId, requestId: idempotencyKey, result: "RETRYABLE" } });
    throw error;
  }
}

export type ContractCapabilities = {
  contractVersion: "1";
  service: { name: string; slug: string };
  operations: Record<string, unknown>;
  bootstrap?: { supported: boolean; creates: string[] };
  resources: Record<string, {
    operations: string[];
    fields?: Record<string, unknown>;
    purge?: { mode: "delete" | "anonymize"; cascades?: string[]; requiresSoleAccountMembership?: boolean };
  }>;
  lifecycle?: { minimumGracePeriodDays?: number };
};

async function getIntegrationClient(integrationId: string) {
  const integration = await db.integration.findUnique({ where: { id: integrationId } });
  if (!integration || !integration.active || integration.revokedAt) {
    throw new Error("SaaS indisponível ou integração revogada.");
  }
  const apiKey = decryptSecret(integration.encryptedApiKey, integration.apiKeyIv, integration.apiKeyAuthTag);
  return { integration, apiKey, endpoint: `${integration.baseUrl}/api/v1/provisioning` };
}

export async function discoverCapabilities(integrationId: string): Promise<ContractCapabilities> {
  const { apiKey, endpoint } = await getIntegrationClient(integrationId);
  let response: Response;
  try {
    response = await fetch(endpoint, {
      headers: { "X-Lynx-API-Key": apiKey },
      cache: "no-store",
      signal: AbortSignal.timeout(12000),
    });
  } catch {
    throw new Error("Não foi possível conectar ao SaaS. Verifique o domínio e a rede.");
  }
  if (!response.ok) throw new Error(`O SaaS recusou a consulta de capacidades (HTTP ${response.status}).`);
  const value: unknown = await response.json().catch(() => null);
  if (!value || typeof value !== "object" || !("contractVersion" in value) || value.contractVersion !== "1" || !("resources" in value)) {
    throw new Error("Este SaaS não publicou capacidades no contrato v1.");
  }
  return value as ContractCapabilities;
}

export async function listResources(
  integrationId: string,
  resourceType: string,
  options: { accountId?: string; lifecycle?: string; search?: string; cursor?: string },
) {
  const capabilities = await discoverCapabilities(integrationId);
  const resource = capabilities.resources[resourceType];
  if (!resource?.operations.includes("list")) throw new Error(`Este SaaS não permite listar ${resourceType}.`);
  const input: Record<string, unknown> = { resourceType };
  if (options.accountId) input.accountId = options.accountId;
  if (options.lifecycle) input.filters = { lifecycle: options.lifecycle };
  if (options.search?.trim()) input.filters = { ...((input.filters as object | undefined) ?? {}), search: options.search.trim() };
  input.page = { limit: 100, ...(options.cursor ? { cursor: options.cursor } : {}) };
  return executeRead(integrationId, { contractVersion: "1", operation: "resource.list", input });
}

async function executeRead(integrationId: string, payload: Record<string, unknown>) {
  const { apiKey, endpoint } = await getIntegrationClient(integrationId);
  let response: Response;
  try {
    response = await fetch(endpoint, {
      method: "POST",
      headers: { "X-Lynx-API-Key": apiKey, "Content-Type": "application/json" },
      body: JSON.stringify(payload),
      cache: "no-store",
      signal: AbortSignal.timeout(30000),
    });
  } catch {
    throw new Error("A solicitação ao SaaS falhou por um problema de rede.");
  }
  const body: unknown = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(readRemoteError(body, response.status));
  return body;
}

export async function executeOperation(
  integrationId: string,
  operation: string,
  input: Record<string, unknown>,
  idempotencyKey: string,
) {
  const capabilities = await discoverCapabilities(integrationId);
  const type = typeof input.resourceType === "string" ? input.resourceType : "";
  const resource = capabilities.resources[type];
  const opName = operation.startsWith("resource.") ? operation.slice("resource.".length) : operation;
  if (operation !== "bootstrap" && !resource?.operations.includes(opName)) {
    throw new Error(`A operação ${opName} não está disponível neste SaaS para ${type}.`);
  }
  const payload = { contractVersion: "1", operation, input };
  const payloadHash = createHash("sha256").update(stableStringify(payload)).digest("hex");
  let record = await db.provisioningRecord.findUnique({ where: { idempotencyKey } });
  if (record && (record.integrationId !== integrationId || record.payloadHash !== payloadHash)) {
    throw new Error("Esta chave de segurança já pertence a outra solicitação.");
  }
  if (record?.responseCiphertext && record.responseIv && record.responseAuthTag) {
    const response = decryptSecret(record.responseCiphertext, record.responseIv, record.responseAuthTag);
    return { status: record.status, body: JSON.parse(response) as unknown };
  }
  if (!record) {
    const encryptedPayload = encryptSecret(JSON.stringify(payload));
    try {
      record = await db.provisioningRecord.create({ data: {
        idempotencyKey,
        integrationId,
        payloadHash,
        payloadCiphertext: encryptedPayload.ciphertext,
        payloadIv: encryptedPayload.iv,
        payloadAuthTag: encryptedPayload.authTag,
        status: "PENDING",
      } });
    } catch {
      record = await db.provisioningRecord.findUnique({ where: { idempotencyKey } });
      if (!record || record.integrationId !== integrationId || record.payloadHash !== payloadHash) {
        throw new Error("Não foi possível reservar esta operação. Atualize a tela e tente novamente.");
      }
      if (record.responseCiphertext && record.responseIv && record.responseAuthTag) {
        const response = decryptSecret(record.responseCiphertext, record.responseIv, record.responseAuthTag);
        return { status: record.status, body: JSON.parse(response) as unknown };
      }
    }
  }

  const { apiKey, endpoint } = await getIntegrationClient(integrationId);
  try {
    const response = await fetch(endpoint, {
      method: "POST",
      headers: {
        "X-Lynx-API-Key": apiKey,
        "Content-Type": "application/json",
        "Idempotency-Key": idempotencyKey,
      },
      body: JSON.stringify(payload),
      cache: "no-store",
      signal: AbortSignal.timeout(30000),
    });
    const body: unknown = await response.json().catch(() => ({}));
    const retryable = response.status === 429 || response.status >= 500;
    const status = retryable ? "RETRYABLE" : response.ok ? "COMPLETED" : `HTTP_${response.status}`;
    const encryptedResponse = retryable ? null : encryptSecret(JSON.stringify(body));
    await db.provisioningRecord.update({ where: { idempotencyKey }, data: {
      status,
      responseCiphertext: encryptedResponse?.ciphertext ?? null,
      responseIv: encryptedResponse?.iv ?? null,
      responseAuthTag: encryptedResponse?.authTag ?? null,
    } });
    await db.auditLog.create({ data: {
      action: operation,
      integrationId,
      requestId: idempotencyKey,
      result: retryable ? `${status}_HTTP_${response.status}` : status,
    } });
    return { status, body };
  } catch (error) {
    await db.provisioningRecord.update({ where: { idempotencyKey }, data: { status: "RETRYABLE" } });
    await db.auditLog.create({ data: { action: operation, integrationId, requestId: idempotencyKey, result: "RETRYABLE" } });
    if (error instanceof Error && error.message.startsWith("A solicitação")) throw error;
    throw new Error("O SaaS não respondeu. A solicitação foi mantida para uma tentativa segura.");
  }
}

function stableStringify(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(",")}]`;
  if (value && typeof value === "object") {
    return `{${Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => `${JSON.stringify(key)}:${stableStringify(item)}`).join(",")}}`;
  }
  return JSON.stringify(value) ?? "null";
}

function readRemoteError(value: unknown, status: number): string {
  if (value && typeof value === "object" && "error" in value) {
    const error = value.error;
    if (error && typeof error === "object" && "message" in error && typeof error.message === "string") return error.message;
    if (typeof error === "string") return error;
  }
  if (value && typeof value === "object" && "message" in value && typeof value.message === "string") return value.message;
  return `O SaaS recusou a operação (HTTP ${status}).`;
}
