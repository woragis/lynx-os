import { createServer } from "node:http";

const port = Number(process.env.LYNX_E2E_AUTH_PORT || 8182);
const csrf = "e2e-csrf-token-which-is-long-enough";
const user = { id: "user-e2e", email: "admin@lynx.test", displayName: "Admin E2E", role: "owner" };
let active = false;
const integrations = [];
const operations = new Map();

function cookies(request) {
  return Object.fromEntries((request.headers.cookie || "").split(";").map((part) => part.trim()).filter(Boolean).map((part) => {
    const split = part.indexOf("=");
    return [part.slice(0, split), part.slice(split + 1)];
  }));
}
function json(response, status, data) {
  response.writeHead(status, { "content-type": "application/json" });
  response.end(JSON.stringify(data));
}
function readBody(request) {
  return new Promise((resolve, reject) => {
    let body = "";
    request.on("data", (chunk) => body += chunk);
    request.on("end", () => resolve(JSON.parse(body || "{}")));
    request.on("error", reject);
  });
}

const server = createServer(async (request, response) => {
  const url = new URL(request.url || "/", `http://localhost:${port}`);
  if (url.pathname === "/health/live") return json(response, 200, { status: "ok" });

  if (url.pathname === "/api/v1/provisioning" && request.method === "GET") {
    if (request.headers["x-lynx-api-key"] !== "e2e-api-key-secret") return json(response, 401, { error: { code: "unauthorized" } });
    return json(response, 200, { contractVersion: "1", service: { name: "SaaS E2E", slug: "saas-e2e" }, operations: {}, resources: {} });
  }

  const jar = cookies(request);
  const csrfMatches = jar.lynx_csrf === csrf && request.headers["x-csrf-token"] === csrf;
  if (url.pathname === "/api/v1/integrations") {
    if (!active || jar.lynx_access !== "e2e-access") return json(response, 401, { error: { code: "unauthorized" } });
    if (request.method === "GET") return json(response, 200, { data: integrations });
    if (request.method === "POST") {
      if (!csrfMatches) return json(response, 403, { error: { code: "csrf_failed" } });
      const body = await readBody(request);
      if (body.apiKey !== "e2e-api-key-secret") return json(response, 400, { error: { code: "invalid_api_key" } });
      const item = { id: `integration-${integrations.length + 1}`, name: body.name, slug: body.slug, baseUrl: body.baseUrl, appUrl: body.appUrl || "", active: true, revokedAt: null };
      integrations.push(item);
      return json(response, 201, { data: item });
    }
  }
  const integrationMatch = url.pathname.match(/^\/api\/v1\/integrations\/([^/]+)$/);
  const integrationNestedMatch = url.pathname.match(/^\/api\/v1\/integrations\/([^/]+)\/operations(?:\/([^/]+)(?:\/(retry))?)?$/);
  if (integrationNestedMatch) {
    if (!active || jar.lynx_access !== "e2e-access") return json(response, 401, { error: { code: "unauthorized" } });
    const integration = integrations.find((item) => item.id === integrationNestedMatch[1]);
    if (!integration || !integration.active) return json(response, 404, { error: { code: "integration_not_found" } });
    const key = integrationNestedMatch[2];
    if (request.method === "GET" && key) {
      const operation = operations.get(`${integration.id}:${key}`);
      if (!operation) return json(response, 404, { error: { code: "operation_not_found" } });
      return json(response, 200, { data: operation });
    }
    if (request.method === "POST" && !key) {
      if (!csrfMatches) return json(response, 403, { error: { code: "csrf_failed" } });
      const body = await readBody(request);
      const idempotencyKey = request.headers["idempotency-key"];
      if (body.operation === "resource.list") return json(response, 200, { data: { contractVersion: "1", requestId: "e2e-read", operation: body.operation, status: "completed", data: { items: [], page: { nextCursor: null } }, replayed: false } });
      if (body.operation !== "bootstrap" || !idempotencyKey) return json(response, 400, { error: { code: "invalid_operation" } });
      const remoteData = {
        account: { id: "account-e2e", state: "created" },
        initialCustomer: { id: "customer-e2e", state: "created" },
        initialUser: { id: "user-resource-e2e", state: "created" },
        invitation: { state: "sent" },
        login: { url: "https://easybill.test/login", email: body.input.initialUser.email },
      };
      const record = { idempotencyKey, status: "completed", operation: body.operation, requestId: idempotencyKey, response: { contractVersion: "1", requestId: "e2e-remote", operation: body.operation, status: "completed", data: remoteData, replayed: false } };
      operations.set(`${integration.id}:${idempotencyKey}`, record);
      return json(response, 200, { data: record.response });
    }
    if (request.method === "POST" && key && integrationNestedMatch[3] === "retry") {
      const operation = operations.get(`${integration.id}:${key}`);
      if (!operation) return json(response, 404, { error: { code: "operation_not_found" } });
      return json(response, 200, { data: operation.response });
    }
  }
  const capabilitiesMatch = url.pathname.match(/^\/api\/v1\/integrations\/([^/]+)\/capabilities$/);
  if (capabilitiesMatch && request.method === "GET") {
    if (!active || jar.lynx_access !== "e2e-access") return json(response, 401, { error: { code: "unauthorized" } });
    const integration = integrations.find((item) => item.id === capabilitiesMatch[1] && item.active);
    if (!integration) return json(response, 404, { error: { code: "integration_not_found" } });
    return json(response, 200, { data: { contractVersion: "1", service: { name: integration.name, slug: integration.slug }, operations: { bootstrap: { supported: true } }, resources: { account: { operations: ["list", "get", "update", "deactivate", "reactivate", "purge"], fields: { editable: ["name", "email"] } }, customer: { operations: ["list", "get", "update", "deactivate", "reactivate", "purge"], fields: { editable: ["tradeName", "email"] } }, user: { operations: ["list", "get", "update", "deactivate", "reactivate", "purge"], fields: { editable: ["name", "email"] } } } } });
  }
  if (integrationMatch) {
    if (!active || jar.lynx_access !== "e2e-access") return json(response, 401, { error: { code: "unauthorized" } });
    const item = integrations.find((integration) => integration.id === integrationMatch[1]);
    if (!item) return json(response, 404, { error: { code: "integration_not_found" } });
    if (request.method === "DELETE") {
      if (!csrfMatches) return json(response, 403, { error: { code: "csrf_failed" } });
      item.active = false;
      item.revokedAt = new Date().toISOString();
      return json(response, 200, { data: item });
    }
    if (request.method === "PUT") {
      if (!csrfMatches) return json(response, 403, { error: { code: "csrf_failed" } });
      const body = await readBody(request);
      Object.assign(item, { name: body.name, slug: body.slug, baseUrl: body.baseUrl, appUrl: body.appUrl || "" });
      return json(response, 200, { data: item });
    }
  }
  const activeMatch = url.pathname.match(/^\/api\/v1\/integrations\/([^/]+)\/active$/);
  if (activeMatch && request.method === "PATCH") {
    if (!active || jar.lynx_access !== "e2e-access" || !csrfMatches) return json(response, 403, { error: { code: "csrf_failed" } });
    const item = integrations.find((integration) => integration.id === activeMatch[1]);
    if (!item) return json(response, 404, { error: { code: "integration_not_found" } });
    item.active = (await readBody(request)).active === true;
    return json(response, 200, { data: item });
  }
  if (url.pathname === "/api/v1/auth/csrf" && request.method === "GET") {
    response.setHeader("set-cookie", `lynx_csrf=${csrf}; Path=/; SameSite=Lax`);
    return json(response, 200, { data: { csrfToken: csrf } });
  }
  if (url.pathname === "/api/v1/auth/login" && request.method === "POST") {
    if (!csrfMatches) return json(response, 403, { error: { code: "csrf_failed" } });
    const body = await readBody(request);
    if (body.email !== user.email || body.password !== "e2e-password-secure") return json(response, 401, { error: { code: "invalid_credentials" } });
    active = true;
    response.setHeader("set-cookie", [
      "lynx_access=e2e-access; Path=/; HttpOnly; SameSite=Lax",
      "lynx_refresh=e2e-refresh; Path=/api/v1/auth; HttpOnly; SameSite=Lax",
      `lynx_csrf=${csrf}; Path=/; SameSite=Lax`,
    ]);
    return json(response, 200, { data: { user, expiresIn: 600, sessionId: "session-e2e" } });
  }
  if (url.pathname === "/api/v1/auth/me" && request.method === "GET") {
    if (!active || jar.lynx_access !== "e2e-access") return json(response, 401, { error: { code: "unauthorized" } });
    return json(response, 200, { data: { user, sessionId: "session-e2e" } });
  }
  if (url.pathname === "/api/v1/auth/sessions" && request.method === "GET") {
    if (!active || jar.lynx_access !== "e2e-access") return json(response, 401, { error: { code: "unauthorized" } });
    return json(response, 200, { data: [{ id: "session-e2e", createdAt: new Date().toISOString(), lastUsedAt: new Date().toISOString(), expiresAt: new Date(Date.now() + 86400000).toISOString(), revokedAt: null, ipAddress: "127.0.0.1", userAgent: "E2E", current: true }] });
  }
  if (url.pathname === "/api/v1/auth/sessions/session-e2e" && request.method === "DELETE") {
    if (!csrfMatches) return json(response, 403, { error: { code: "csrf_failed" } });
    active = false;
    return json(response, 200, { data: { status: "revoked" } });
  }
  if (url.pathname === "/api/v1/auth/logout" && request.method === "POST") {
    if (!csrfMatches) return json(response, 403, { error: { code: "csrf_failed" } });
    active = false;
    response.setHeader("set-cookie", ["lynx_access=; Path=/; Max-Age=0", "lynx_refresh=; Path=/api/v1/auth; Max-Age=0", "lynx_csrf=; Path=/; Max-Age=0"]);
    return json(response, 200, { data: { status: "signed_out" } });
  }
  return json(response, 404, { error: { code: "not_found" } });
});

server.listen(port);
