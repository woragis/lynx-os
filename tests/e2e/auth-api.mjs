import { createServer } from "node:http";

const port = Number(process.env.LYNX_E2E_AUTH_PORT || 8182);
const csrf = "e2e-csrf-token-which-is-long-enough";
const user = { id: "user-e2e", email: "admin@lynx.test", displayName: "Admin E2E", role: "owner" };
let active = false;

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
