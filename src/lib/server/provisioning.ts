import "server-only";

import { createHash } from "node:crypto";
import { db } from "@/lib/server/db";
import { decryptSecret, encryptSecret } from "@/lib/server/crypto";

export type BootstrapPayload = {
  contractVersion: "1";
  operation: "bootstrap";
  account: { name: string; taxId: string; email: string; address: string };
  initialCustomer: { tradeName: string; legalName: string; taxId: string; email: string; phone: string; taxRegime: string; relationshipType: string; status: string };
  initialUser: { name: string; email: string; phone: string; role: "owner"; invite: true };
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
