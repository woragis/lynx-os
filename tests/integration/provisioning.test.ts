import { afterAll, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { createServer, type Server } from "node:http";
import { once } from "node:events";
import { encryptSecret } from "@/lib/server/crypto";
import { db } from "@/lib/server/db";
import { executeProvisioning, type BootstrapPayload } from "@/lib/server/provisioning";

const payload: BootstrapPayload = {
  contractVersion: "1", operation: "bootstrap",
  account: { name: "Escritório Aurora", taxId: "12345678000199", email: "admin@aurora.test", address: "Rua das Flores, 10" },
  initialCustomer: { tradeName: "Cliente Aurora", legalName: "Aurora Ltda", taxId: "98765432000199", email: "cliente@aurora.test", phone: "11999990000", taxRegime: "Simples Nacional", relationshipType: "Recorrente", status: "Ativo" },
  initialUser: { name: "Ana Aurora", email: "ana@aurora.test", phone: "11988880000", role: "owner", invite: true },
};

let server: Server;
let baseUrl: string;
let postCount = 0;
let nextPostStatus = 201;
let responseBody: unknown;
const apiKey = "api-key-de-integracao";

beforeAll(async () => {
  process.env.LYNX_ENCRYPTION_KEY = "c".repeat(64);
  server = createServer(async (request, response) => {
    if (request.headers["x-lynx-api-key"] !== apiKey) {
      response.writeHead(401).end(JSON.stringify({ error: "unauthorized" }));
      return;
    }
    if (request.method === "GET") {
      response.writeHead(200, { "content-type": "application/json" }).end(JSON.stringify({ contractVersion: "1" }));
      return;
    }
    postCount += 1;
    let body = "";
    for await (const chunk of request) body += chunk;
    responseBody = JSON.parse(body);
    response.writeHead(nextPostStatus, { "content-type": "application/json" }).end(JSON.stringify({ id: "tenant-1", received: true }));
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("Servidor de teste sem porta.");
  baseUrl = `http://127.0.0.1:${address.port}`;
});

afterAll(async () => {
  await db.$disconnect();
  server.close();
});

beforeEach(async () => {
  await db.auditLog.deleteMany();
  await db.provisioningRecord.deleteMany();
  await db.integration.deleteMany();
  postCount = 0;
  nextPostStatus = 201;
  responseBody = undefined;
});

async function createIntegration(active = true) {
  const encrypted = encryptSecret(apiKey);
  return db.integration.create({ data: {
    name: "EasyBill", slug: "easybill", baseUrl,
    encryptedApiKey: encrypted.ciphertext, apiKeyIv: encrypted.iv, apiKeyAuthTag: encrypted.authTag, active,
  } });
}

describe("executeProvisioning com SQLite e SaaS HTTP de teste", () => {
  it("consulta capacidades, envia contrato e protege o resultado idempotente", async () => {
    const integration = await createIntegration();
    const result = await executeProvisioning(integration.id, payload, "request-success-1");
    const repeated = await executeProvisioning(integration.id, payload, "request-success-1");

    expect(result).toEqual({ status: "COMPLETED", body: { id: "tenant-1", received: true } });
    expect(repeated).toEqual(result);
    expect(postCount).toBe(1);
    expect(responseBody).toEqual(payload);
    expect(await db.auditLog.count()).toBe(1);
    const record = await db.provisioningRecord.findUniqueOrThrow({ where: { idempotencyKey: "request-success-1" } });
    expect(record.responseCiphertext).not.toContain("tenant-1");
  });

  it("impede reutilizar idempotency key com outro payload", async () => {
    const integration = await createIntegration();
    await executeProvisioning(integration.id, payload, "request-conflict-1");
    await expect(executeProvisioning(integration.id, { ...payload, account: { ...payload.account, name: "Outro" } }, "request-conflict-1"))
      .rejects.toThrow(/Idempotency-Key já foi usada/);
    expect(postCount).toBe(1);
  });

  it("não faz chamadas para integrações pausadas", async () => {
    const integration = await createIntegration(false);
    await expect(executeProvisioning(integration.id, payload, "request-paused-1")).rejects.toThrow(/pausada/);
    expect(postCount).toBe(0);
  });

  it("marca falha transitória para permitir retry com a mesma chave", async () => {
    const integration = await createIntegration();
    nextPostStatus = 503;
    const result = await executeProvisioning(integration.id, payload, "request-retry-1");
    expect(result.status).toBe("RETRYABLE");
    expect(await db.provisioningRecord.findUniqueOrThrow({ where: { idempotencyKey: "request-retry-1" } }).then((record) => record.status)).toBe("RETRYABLE");
    expect(await db.auditLog.findFirstOrThrow()).toMatchObject({ result: "RETRYABLE_HTTP_503" });
  });
});
