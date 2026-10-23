import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("next/headers", () => ({ cookies: async () => ({
  getAll: () => [{ name: "lynx_access", value: "session-cookie" }, { name: "lynx_csrf", value: "csrf-cookie" }],
  get: (name: string) => name === "lynx_csrf" ? { value: "csrf-cookie" } : undefined,
}) }));

import { executeOperation, executeProvisioning, type BootstrapPayload } from "@/lib/server/provisioning";

const payload: BootstrapPayload = {
  contractVersion: "1", operation: "bootstrap",
  input: {
    account: { name: "Escritório Aurora", taxId: "12345678000199", email: "admin@aurora.test", address: "Rua das Flores, 10" },
    initialCustomer: { tradeName: "Cliente Aurora", legalName: "Aurora Ltda", taxId: "98765432000199", email: "cliente@aurora.test", phone: "11999990000", taxRegime: "Simples Nacional", relationshipType: "Recorrente", status: "Ativo" },
    initialUser: { name: "Ana Aurora", email: "ana@aurora.test", phone: "11988880000" },
  },
};

afterEach(() => { vi.unstubAllGlobals(); });

describe("adaptador Next para a API Go", () => {
  it("envia bootstrap e idempotência ao Go e traduz a resposta do contrato", async () => {
    process.env.LYNX_GO_INTERNAL_URL = "http://lynx-go.test/";
    const fetchMock = vi.fn(async (_url: string | URL | Request, init?: RequestInit) => {
      expect(String(_url)).toBe("http://lynx-go.test/api/v1/integrations/saas-1/operations");
      const headers = new Headers(init?.headers);
      expect(headers.get("cookie")).toContain("lynx_access=session-cookie");
      expect(headers.get("x-csrf-token")).toBe("csrf-cookie");
      expect(headers.get("idempotency-key")).toBe("request-key-001");
      expect(JSON.parse(String(init?.body))).toEqual({ operation: "bootstrap", input: payload.input });
      return Response.json({ data: { contractVersion: "1", status: "completed", data: { data: { id: "tenant-1", login: { url: "https://easybill.test/login", email: "ana@aurora.test" } } } } });
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await executeProvisioning("saas-1", payload, "request-key-001");
    expect(result).toEqual({ status: "COMPLETED", body: { data: { id: "tenant-1", login: { url: "https://easybill.test/login", email: "ana@aurora.test" } } } });
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it("encaminha operações de gestão sem acesso local ao SQLite", async () => {
    process.env.LYNX_GO_INTERNAL_URL = "http://lynx-go.test";
    const fetchMock = vi.fn(async (_url: string | URL | Request, init?: RequestInit) => {
      expect(new Headers(init?.headers).get("idempotency-key")).toBe("resource-update-01");
      expect(JSON.parse(String(init?.body))).toEqual({ operation: "resource.update", input: { resourceType: "customer", resourceId: "c-1", accountId: "a-1", changes: { tradeName: "Novo nome" } } });
      return Response.json({ data: { status: "completed", data: { data: { id: "c-1", state: "updated" } } } });
    });
    vi.stubGlobal("fetch", fetchMock);
    const result = await executeOperation("saas-1", "resource.update", { resourceType: "customer", resourceId: "c-1", accountId: "a-1", changes: { tradeName: "Novo nome" } }, "resource-update-01");
    expect(result).toEqual({ status: "COMPLETED", body: { data: { id: "c-1", state: "updated" } } });
  });
});
