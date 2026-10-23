import "server-only";

import { lynxApi } from "@/lib/server/lynx-api";

export type BootstrapPayload = {
  contractVersion: "1";
  operation: "bootstrap";
  input: {
    account: { name: string; taxId: string; email: string; address: string };
    initialCustomer: { tradeName: string; legalName: string; taxId: string; email: string; phone: string; taxRegime: string; relationshipType: string; status: string };
    initialUser: { name: string; email: string; phone: string };
  };
};

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

type ApiResponse<T> = { data: T };
type IntegrationResponse = { contractVersion: string; requestId: string; operation: string; status: string; data: unknown; replayed: boolean };

export async function discoverCapabilities(integrationId: string): Promise<ContractCapabilities> {
  const result = await lynxApi<ApiResponse<ContractCapabilities>>(`/integrations/${encodeURIComponent(integrationId)}/capabilities`);
  return result.data;
}

export async function listResources(integrationId: string, resourceType: string, options: { accountId?: string; lifecycle?: string; search?: string; cursor?: string }) {
  const input: Record<string, unknown> = { resourceType, page: { limit: 100, ...(options.cursor ? { cursor: options.cursor } : {}) } };
  if (options.accountId) input.accountId = options.accountId;
  const filters: Record<string, string> = {};
  if (options.lifecycle) filters.lifecycle = options.lifecycle;
  if (options.search?.trim()) filters.search = options.search.trim();
  if (Object.keys(filters).length) input.filters = filters;
  const result = await lynxApi<ApiResponse<IntegrationResponse>>(`/integrations/${encodeURIComponent(integrationId)}/operations`, { method: "POST", body: JSON.stringify({ operation: "resource.list", input }) });
  return result.data.data;
}

export async function executeOperation(integrationId: string, operation: string, input: Record<string, unknown>, idempotencyKey: string) {
  const result = await lynxApi<ApiResponse<IntegrationResponse>>(`/integrations/${encodeURIComponent(integrationId)}/operations`, {
    method: "POST", headers: idempotencyKey ? { "Idempotency-Key": idempotencyKey } : undefined,
    body: JSON.stringify({ operation, input }),
  });
  return { status: result.data.status.toUpperCase(), body: result.data.data };
}

export async function executeProvisioning(integrationId: string, payload: BootstrapPayload, idempotencyKey: string) {
  return executeOperation(integrationId, payload.operation, payload.input, idempotencyKey);
}

export async function getOperation(integrationId: string, idempotencyKey: string) {
  const result = await lynxApi<ApiResponse<{ idempotencyKey: string; status: string; operation: string; requestId: string; response?: unknown }>>(
    `/integrations/${encodeURIComponent(integrationId)}/operations/${encodeURIComponent(idempotencyKey)}`,
  );
  return result.data;
}

export async function listIntegrations<T = { id: string; name: string; baseUrl: string; appUrl?: string; active: boolean; revokedAt?: string }>() {
  const result = await lynxApi<ApiResponse<T[]>>("/integrations");
  return result.data;
}
