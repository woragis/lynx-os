# CHECKME / Brismarp backend playbook

Use this file as the source of truth for **this** backend and as a template for
future Go APIs. Paste it into an AI context and follow it. Do not reopen
decisions that are marked **locked**.

Audience: humans and coding agents.

---

## How an AI must use this document

1. Follow this file to the letter. Do not propose Fiber, SQLite, AutoMigrate, or
   `pacienteId` taken from the client body.
2. After every finished slice, **git commit** (see Git). Never batch a whole day
   without commits.
3. Never add `Co-authored-by: Cursor` (or any Cursor/tool co-author trailer).
4. Domain and application layers must not import chi, GORM, pgx, or go-redis.
5. Redis is used **from application services**, only through ports defined in
   domain. Never inject `*redis.Client` into a service.
6. One HTTP binary now. gRPC is a future `cmd/grpc` that injects the **same**
   application services.

---

## Goal

A **modular monolith**: Clean Architecture + DDD, PostgreSQL as source of truth,
Redis for short-lived runtime concerns, HTTP today and gRPC later without
rewriting use cases.

This CHECKME block (product scope) is: identity, RBAC, patient, professional,
agenda, persisted CMS/plans, subscription/invoice **stubs**, coupons, affiliates,
minimal admin, partner directory for in-person care with city and verified-coordinate search,
online physician suggestions by specialty and earliest free slot (patient confirms before booking),
optional triage bound to the authenticated user, **email verification and
password reset**, **agenda and grave-triage notifications** (same Mailgun +
WhatsApp ports, best-effort — booking must not fail if notify is down),
**professional portal** (agenda, slots, confirm, **SOAP of the linked consult**) and **document upload**,
**audit log** of clinical access and admin actions (no SOAP body in the row),
**LGPD of the authenticated titular** (explicit consent on register, ficha
`PATCH /me`, `GET /me/export` JSON, `POST /me/erase` anonymize), **SOAP/history
AES-256-GCM at rest** (`FIELD_ENCRYPTION_KEY`), **deterministic triage
overrides** (status schema + Go red flags → `ALERTA_GRAVE`), and **videoconsulta
Jitsi** (`VIDEO_BASE_URL`; Go creates the room and records join/end — it does
**not** carry audio/video).

Billing checkout is **skipped** until the client names a payment provider.

**Out of this block:** payment gateway, webhooks, **2FA**, video **recording**,
LiveKit/Daily/Twilio/Zoom, real homolog/production environments, backup, CI, TLS in the Go binary,
Manchester protocol as a closed clinical product,
Grafana/Alertmanager/pager. Prometheus scrape
(`GET /metrics`), `/health/ready`, **OpenTelemetry traces** and **Jaeger local**
**are** in this block.

The previous FastAPI app under `backend/app/` is **reference only** (triage
prompt, `content.py` seed data, JSON shapes). Production traffic is Go.

---

## Dependency rule (locked)

```
interfaces → application → domain ← infrastructure
```

Composition root is **only** `cmd/`. Modules do not self-register via `init()`.

| Piece | Lives in | May import | Must not import |
|---|---|---|---|
| Shared errors | `internal/domain/apperr` | stdlib | chi, GORM, pgx, redis |
| Entity / VO | `internal/domain/<ctx>/entity` | stdlib, other domain pkgs, `apperr` | chi, GORM, pgx, redis, proto |
| Repository interface | `internal/domain/<ctx>/repository` | domain entities | infrastructure, chi |
| Runtime ports (Redis) | `internal/domain/<ctx>/port` or next to repository | domain entities | `go-redis` |
| Application service | `internal/application/<ctx>` | domain | chi, GORM, `go-redis`, proto |
| SQL/GORM/Redis adapters | `internal/infrastructure/...` | domain interfaces + drivers | chi handlers |
| HTTP handler | `internal/interfaces/http/<ctx>` | application services, DTOs | GORM, sqlc, redis client |
| HTTP routes | same, `routes.go` | chi, handlers | business rules |
| gRPC (future) | `internal/interfaces/grpc/<ctx>` | **same** application services | chi |

**Handler** maps HTTP JSON ↔ use-case input/output. **Routes** only mount
`http.Handler` on a `chi.Router`. **Service** orchestrates repositories and Redis
ports. Failures are `*apperr.Error` from the owning layer; handlers call
`httpx.WriteError` once and never stringify `err.Error()` to the client.

---

## Folder tree (locked)

```
cmd/http/main.go              # composition root: chi, pgx, GORM, Redis, wiring
cmd/grpc/main.go              # future; same services

internal/platform/            # kernel, not domain
  config/                     # caarlos0/env
  postgres/                   # pgx pool + *gorm.DB
  redis/                      # go-redis client constructor
  httpx/                      # chi factory, JSON errors, auth middleware
  authn/                      # JWT access sign/verify
  metrics/                    # Prometheus counters/histograms; chi route templates
  otelx/                      # OTLP tracer (no-op if endpoint empty); slog + trace_id

internal/domain/
  apperr/                     # Code catalog, *Error, Is/Wrap/At
  identity/entity/
  identity/repository/
  identity/port/              # RefreshTokenStore, OTPStore
  notify/port/                # Mailer, WhatsApp (no HTTP)
  patient/...
  professional/...
  agenda/entity/
  agenda/repository/
  agenda/port/                # SlotLocker, IdempotencyStore
  cms/...
  billing/...
  coupon/...
  affiliate/...
  triage/...
  video/...

internal/application/
  identity/
  patient/
  professional/
  agenda/
  cms/
  billing/
  coupon/
  affiliate/
  triage/
  video/

internal/infrastructure/
  postgres/sqlc/              # nucleus: identity, patient, professional, agenda, triage
  postgres/gorm/              # admin CRUDs: cms, coupons, affiliates, billing, teleconsultations
  redis/                      # implements ports
  anthropic/                  # triage LLM client
  mailgun/                    # Mailer (HTTP API, not SMTP)
  whatsapp/meta/              # Cloud API templates
  whatsapp/zapi/              # send-text fallback
  notify/                     # log Mailer/WhatsApp when no credentials

internal/interfaces/
  http/identity/              # handler + routes
  http/agenda/
  http/cms/
  ...
  grpc/identity/              # empty until needed

backend/migrations/           # golang-migrate SQL only
```

Bounded contexts: **identity**, **notify**, **patient**, **professional**,
**agenda**, **cms**, **billing**, **coupon**, **affiliate**, **triage**, **video**.

---

## Redis in services (locked)

Postgres is the source of truth for people, money, and clinical text.

Application services **do** use Redis, by depending on ports:

| Service | Port | When |
|---|---|---|
| IdentityService | `RefreshTokenStore` | login stores refresh; refresh rotates; logout deletes; reset deletes all for user |
| IdentityService | `OTPStore` | 6-digit codes hashed, TTL ~15 min; fail closed if Redis is down |
| IdentityService | `notify.Mailer`, `notify.WhatsApp` | verify + reset; application never imports Mailgun/Meta/Z-API |
| CmsService | `Cache` | public GETs read cache; admin writes `Del` keys |
| AgendaService | `SlotLocker`, `IdempotencyStore` | lock slot before INSERT; duplicate POST key |
| TriageService | `RateLimiter` | max messages per minute per `user_id` |

Flow: handler → service → port. Adapter in `infrastructure/redis` talks to Redis
(`SET NX` + TTL for locks, string keys for refresh, etc.).

If Redis is down:

- Cache miss → read Postgres.
- Lock failure → do not book; return explicit error (503/retry), never a silent
  double-book. Postgres unique/exclusion is the second line of defence.
- Refresh store down → login/refresh/logout fail closed (never fake a session).
- OTP store down → verify / forgot / reset fail closed (`redis.unavailable`).
- Mailer/WhatsApp down on **resend** → `notify.unavailable`. Forgot-password still
  returns 204 (no account enumeration); log the send failure.

**Never** cache SOAP, medical history, invoices, or patient profile in Redis.

Client: `github.com/redis/go-redis/v9`. Service tests: in-memory fakes of ports.
Adapter tests: miniredis.

---

## Packages (locked)

| Concern | Choice | Notes |
|---|---|---|
| HTTP | **chi v5** (`github.com/go-chi/chi/v5`) | `chi.Router` is `http.Handler`. Handlers are stdlib. Tests use `httptest`. |
| SQL nucleus | **pgx** + **sqlc** | Identity, patients, professionals, agenda, triage. |
| SQL admin CRUD | **GORM** | CMS, coupons, affiliates, billing, teleconsultations. Map to domain entities; **no** `gorm.Model` on domain types. |
| Schema | **golang-migrate** SQL in `backend/migrations/` | Not GORM AutoMigrate. Not sqlc schema. |
| Redis | **go-redis/v9** | Only in platform/infrastructure. |
| Password | `golang.org/x/crypto/bcrypt` | |
| Access token | `github.com/golang-jwt/jwt/v5` | Short-lived JWT. Refresh is **opaque** in Redis. |
| HTTP DTO validation | `github.com/go-playground/validator/v10` | Tags on HTTP DTOs only. |
| Config | `github.com/caarlos0/env/v11` | `DATABASE_URL` and `REDIS_URL` required. |
| Log | `log/slog` JSON | `request_id` + `trace_id`/`span_id` from context |
| Tracing | OpenTelemetry + OTLP HTTP | Empty `OTEL_EXPORTER_OTLP_ENDPOINT` → no-op. Jaeger local. |
| IDs | `github.com/google/uuid` | |
| Tests | `testing` + `testify/require` + `httptest` | |
| Email | **Mailgun HTTP API** | Not SMTP. Adapter in infrastructure. Empty key → log adapter. |
| WhatsApp | **porta `notify.WhatsApp`** | `cmd/` picks Meta Cloud if fully configured, else Z-API, else log. Never wait for Meta. |

Compose: `postgres:16`, `redis:7`, **Jaeger all-in-one** (OTLP 4318, UI 16686). No SQLite.

Go version: 1.22+ (1.24 fine). Module path: `github.com/brismarp/checkme-backend` if inside the backend repo, or the module path already in `go.mod`.

---

## Rejected (do not revive)

| Rejected | Why |
|---|---|
| Fiber / fasthttp | Not `http.Handler`. Breaks gRPC-gateway, OTel, `httptest` style. |
| Gin / Echo | Fine libraries; chi stays closer to stdlib for this template. |
| ServeMux-only | Valid, more verbose for groups. **Locked to chi.** |
| sqlc everywhere | Admin CMS CRUDs become tedious. Hybrid is locked. |
| GORM on domain structs | Leaks ORM into DDD. Adapters map. |
| AutoMigrate as schema | Not versioned. Use golang-migrate. |
| SQLite | Production path is PostgreSQL. |
| Client-supplied `pacienteId` | IDOR. Triage uses authenticated user / patient FK. |
| 2FA | Later block. Email verify + password reset are in this block. |
| SMTP (Hostinger or Mailgun SMTP) | Mailgun **HTTP** only. Hostinger may be an inbox, not the sender. |
| Waiting on Meta WhatsApp before shipping | Z-API or log until Meta env is complete. Both adapters ship now. |
| Payment gateway / webhooks | Later block. Hire activates the plan; invoices stay stub until a gateway. |
| Microservices | One HTTP process. |

---

## PostgreSQL model (this product)

- `users` — email unique, password_hash, nome, role (`paciente\|profissional\|afiliado\|admin`), active, `email_verified_at` (null until verified)
- `patients` — user_id, cpf, phone, lgpd_consent_at
- `dependents` — patient_id, nome, cpf, kinship
- `professionals` — user_id, council, council_number, specialties, status (`pendente\|aprovado\|reprovado`)
- `plans`, `specialties`, `discounts`, `faq_items`, `contact` — seed from former `content.py`
- `availability_slots`, `appointments` — status `reservada\|confirmada\|cancelada\|reagendada`; prevent professional overlap; optional `triage_session_id` (completed session of the titular)
- `teleconsultations` — one row per appointment (`appointment_id` unique); `room_slug` (`checkme-{uuid}`); status `aberta\|em_curso\|encerrada`; `started_at` / `ended_at`. **No media bytes.**
- `subscriptions`, `invoices` — hire creates `ativa` / invoice `aberta`; no gateway IDs
- `coupons`, `coupon_redemptions`
- `affiliates`, `affiliate_attributions`
- `triage_sessions` — FK to patient/user; `history_json` and `soap` encrypted at rest (`enc:v1:` AES-256-GCM) when `FIELD_ENCRYPTION_KEY` is set; plaintext legacy rows still read; **SOAP: owning patient or the professional of a linked non-cancelled appointment**
- `audit_logs` — actor, action, resource id, result, request id; **never** SOAP or file bytes

Seed: admin, sample patient, sample approved professional, four plans.

---

## HTTP API (`/api/v1`)

Process-level (not under `/api/v1`):

| Path | Who | What |
|---|---|---|
| `GET /health` | public | liveness (process up) |
| `GET /health/ready` | public | Postgres + Redis ping; 503 `postgres.unavailable` / `redis.unavailable` |
| `GET /metrics` | scrape | Prometheus; off when `METRICS_ENABLED=false`. No PII. |

| Prefix | Who | What |
|---|---|---|
| `/auth` | public | register (`lgpdConsent: true` required → else `identity.consent_required`), login (10/min per IP), refresh, logout, forgot-password, reset-password |
| `/auth/verify-email`, `/auth/resend-verification` | authenticated | 6-digit OTP |
| `/me` | titular | profile (`cpf`, `phone`, `lgpdConsentAt`), `PATCH` ficha, dependents (`POST`, `PATCH`/`DELETE /me/dependents/{id}` own only), subscription, triage list, **`GET /me/triagens/{id}`** (history + SOAP), **`GET /me/export`**, **`POST /me/erase`** |
| `/professionals` | approved professional / admin | profile, slots, documents (PDF/JPEG/PNG ≤ 5 MB) |
| `/appointments` | patient + professional | create (`sessaoId` optional — must be the titular's terminal session; else attach latest completed), list (`triageSessionId`, `triageStatus`, `videoStatus`; **never SOAP**, **never video URL**), cancel, reschedule, confirm |
| `/appointments/{id}/video/join` | titular of that consult **or** professional of that consult (**not** admin) | `{ url, roomSlug, displayName, role }` (`paciente`\|`profissional`). URL = `{VIDEO_BASE_URL}/{roomSlug}` + Jitsi hash **without** recording/livestream. First join: `aberta` → `em_curso` + `started_at`. Cancelled consult → `video.forbidden`. Ended room → `video.ended`. |
| `/appointments/{id}/video/end` | same as join | `encerrada` + `ended_at`. Cancel appointment also ends an `aberta`/`em_curso` room. |
| `/planos`, `/especialidades`, `/rede-credenciada/descontos`, `/faq`, `/contato` | public GET; admin write (`PUT /admin/planos/{id}`, `PUT /admin/especialidades`, `PUT /admin/descontos`, `PUT /admin/faq`, `PUT /admin/contato`) | CMS |
| `/subscriptions` | patient | create `ativa` (cancels previous open sub); optional `couponCode` → discount + redeem |
| `/coupons` | admin create/list/`PATCH` active; `POST /coupons/validate` authenticated | |
| `/affiliates` | admin create/list (`commissionPercent`); optional referral code on register (uppercase) | |
| `/admin` | role admin | users (list, **export**, **erase**, set active), professional approval, CMS, coupons, affiliates, appointments, **`GET /admin/billing`** (subs, invoices, commissions), **`POST /admin/billing/invoices/{id}/confirm-stub`** (fatura `paga` + comissão `lancada`; plano já está `ativa` no contratar), **audit log** |
| `/chat`, `/soap/{id}` | owning patient; SOAP also the professional of a linked consult | same product contract as the old API, with auth. Terminal session (`LEVE\|MODERADO\|ALERTA_GRAVE`) is frozen: a later `POST /chat` with that `sessaoId` opens a **new** session. |

Auth: `Authorization: Bearer <access>`. Refresh is opaque in Redis. Password
reset invalidates every refresh for that user.

OTP: 6 digits, both email and WhatsApp. Templates are IDs (`email.verify`,
`email.reset`, `whatsapp.verify`, `whatsapp.reset`). Never put the code in a URL.

Product events reuse the same ports, best-effort (log + continue):

- Agenda book / cancel / reschedule / confirm → patient and professional
  (`email.agenda.*` / `whatsapp.agenda.*`).
- First `ALERTA_GRAVE` on a triage session (including a Go red-flag override) →
  patient, plus CMS contact WhatsApp as ops (`email.triage.grave` /
  `whatsapp.triage.grave`). Do not put SOAP in the message.
- Account created (best-effort, in addition to the OTP) and e-mail verified
  (`email.account.*` / `whatsapp.account.*`).
- Professional approved / rejected on admin `SetStatus`
  (`email.professional.*` / `whatsapp.professional.*`).
- Payment events stay out until there is a gateway.

Triage after the model: status must be
`ALERTA_GRAVE|MODERADO|LEVE|CONTINUAR_TRIAGEM` (else fallback continue, SOAP
nil). Red flags in `internal/domain/triage/redflag` (normalized pt-BR, user
messages only) force `ALERTA_GRAVE` and a fixed emergency copy. This is **not**
Manchester and **not** a physician review of the classification. SOAP readers
in this block (B-52 locked here): **the titular** and **the professional of
the linked consult**. Admin, plantão, and every professional are out. Never
put SOAP on the appointment JSON.

`POST /me/erase` anonymizes the titular (inactive user, generic e-mail/nome,
cleared CPF/phone/dependents/SOAP/history/appointment notes). `audit_logs`
remain. Appointments stay as operational history under “Titular removido”.
Admin DSAR is the same contract: `GET /admin/users/{id}/export` and
`POST /admin/users/{id}/erase`. Admin cannot erase self or `role=admin`.
SOAP is in the export file, never on the admin table.

Audit writes are best-effort (same as product notify): a down insert is logged and the
request still returns. Rows cover login, SOAP read, document upload/download,
agenda mutations, professional status, plan upsert, admin user list,
`patient.profile.update`, `patient.export`, `patient.erase`, `video.join`,
`video.end`.
`GET /admin/audit` is admin-only. Never store SOAP text or document bytes.

Meta WhatsApp only sends OTP templates. Agenda/grave WhatsApp goes out on Z-API
or the log adapter until Meta templates exist. Email is always composed as text.

Login without verified email is allowed. `POST /appointments` and
`POST /subscriptions` require `email_verified_at` (`identity.unverified`).
Triage chat does not. Optional `couponCode` on subscribe validates, discounts
the invoice, and writes `coupon_redemptions`. Referral on register (uppercase
lookup) writes `affiliate_attributions`. Subscribe activates the plan (`ativa`),
cancels any previous open subscription, and projects `affiliate_commissions`
(`projetada`). `GET /me` returns the active subscription with `planNome`.
Admin `POST /admin/billing/invoices/{id}/confirm-stub` marks the invoice `paga`
and commissions `lancada` without changing the subscription. Real gateway and
refunds stay out.

Videoconsulta: the Go API does **not** transport audio/video. Booking creates
`teleconsultations` (`aberta`, slug `checkme-{uuid}`). Join is the titular or
that consult's professional only (same gate as SOAP; **admin is out**). Media
lives on the Jitsi host in `VIDEO_BASE_URL` (default `https://meet.jit.si`).
Swap host = env + restart, same spirit as WhatsApp. No recording, no
livestream, no room JWT, no TURN, no LiveKit/Daily/Twilio/Zoom in this
code. Evidence: `started_at` on first join, `ended_at` on end, `audit_logs`
(`video.join` / `video.end`). `meet.jit.si` in local is a third-party
processor until production points `VIDEO_BASE_URL` at own Jitsi.

WhatsApp wiring (**locked**, composition root only):

1. Meta Cloud complete (`META_WA_TOKEN`, `META_WA_PHONE_ID`, both template names) → Meta.
2. Else Z-API complete → Z-API `send-text`.
3. Else log adapter.

Upgrade to official WhatsApp is env + process restart. Application code does not
change. Meta uses approved templates (`{{1}}` = code). Z-API interpolates the
canonical pt-BR sentence. Forgot-password always HTTP 204. WhatsApp is sent only
when `patients.phone` is set.

Never log OTP plaintext except the local log adapter (so developers can copy the
code). Production adapters must not log `vars["code"]`.

---

## Errors (locked)

Two audiences, two channels.

| Who | Needs | Where |
|---|---|---|
| Mobile / admin / i18n | **why** (stable) | JSON `error.code` |
| Engineer / support | **where** (layer + function + cause) | `slog` + chi `X-Request-Id` |

JSON (always):

```json
{ "error": { "code": "identity.email_taken", "message": "Este e-mail já está cadastrado" } }
```

Validation:

```json
{
  "error": {
    "code": "http.validation",
    "message": "Dados inválidos",
    "fields": [{ "field": "email", "code": "http.validation.email" }]
  }
}
```

- `code` — English dotted key. Immutable. i18n key (`errors.{code}`). Tests assert this, never `message` substrings.
- `message` — pt-BR fallback until the client has `t(code)`. Clients must not parse the string for logic.
- **Never** in JSON: stack, SQL, `src`, file path, driver text.

Log (always, inside `httpx.WriteError`): `code`, `http`, `src`, `cause`, `request_id`.

### Uniqueness (three rules, not one)

1. Each `Code` is **defined once** in `internal/domain/apperr/codes.go` (HTTP status + pt-BR message in one table).
2. HTTP mapping is **once**: every handler calls `httpx.WriteError(w, r, err)`. No `httpx.Error(w, 400, "invalid", err.Error())`.
3. The **owning layer** constructs that business error. Handlers do not reinterpret `"forbidden"`.

Do **not** invent a code per route for the same meaning (`login.invalid_json` vs `register.invalid_json`). The route is already in the URL + request id.

Prefix = conceptual place:

| Prefix | Owner |
|---|---|
| `http.*` | transport (JSON, validator, bearer) |
| `identity.*` `agenda.*` `triage.*` `notify.*` … | application / domain rule |
| `redis.*` | Redis port failed (fail-closed) |
| `postgres.*` | unexpected SQL (not a domain conflict) |
| `internal` | unknown; never leak `cause` in JSON |

Construction:

```go
return apperr.New(apperr.IdentityEmailTaken).At("identity.service.Register")
return apperr.Wrap(err, apperr.PostgresUnavailable).At("sqlc.User.Create")
```

`WriteError`: `errors.As` → `*apperr.Error`; else log + `internal`. Auth middleware uses `http.unauthorized` / `http.forbidden` with `.At("httpx.BearerAuth")`.

Overlap: Postgres exclusion → `agenda.slot_taken` (409). Redis lock busy or Redis down on lock → `agenda.lock_unavailable` (503). Refresh store down → `redis.unavailable` (503), never fake a session.

No `Accept-Language` in this block. `code` is the i18n contract.

---

## Git / commits (locked)

Applies to this repo, this playbook, and future projects that copy this file.

- **Always commit** each finished slice (docs, feature, test, chore). Do not leave a full day uncommitted.
- **Conventional Commits** subject line. Optional body via HEREDOC.
- Allowed types: `feat:`, `fix:`, `docs:`, `test:`, `chore:`, `refactor:`, `build:`, `ci:`.
- There is no `etc:` type. If unsure, use `chore:`.
- Optional scope: `feat(identity):`, `feat(agenda):`, `docs:`.
- **Forbidden:** `Co-authored-by: Cursor`, `Co-authored-by: cursor`, `coauthor: cursor`, or any tool/AI co-author trailer. The commit author is the local git user only.
- Do not `--no-verify` unless the human asked. Do not change `git config`. Do not commit `.env` or secrets.
- Example subjects: `docs: record backend architecture playbook`, `feat(identity): add jwt login and redis refresh store`, `test(agenda): reject overlapping slots`.

---

## Testing (locked)

Assert **`error.code`**, never a substring of `message`.

| Layer | How | Command |
|---|---|---|
| Catalog | every `Code` unique, has HTTP + message | `go test ./internal/domain/apperr` |
| Application | fake repos + fake Redis ports; `apperr.Is(err, code)` | `go test ./...` |
| HTTP | `httptest` + chi; helper `requireAPIError(t, rec, status, code)` | same |
| Redis adapter | miniredis | same |
| Postgres adapter | `//go:build integration` + `DATABASE_URL` | `make test-int` |
| Live stack (opt-in) | Playwright: Vite panel `:5173`, Expo web `:8081`, request tests against Go `:8000` | `make test-e2e` |

HTTP must cover: invalid JSON → `http.invalid_json`; missing bearer → `http.unauthorized`; paciente on `/admin` → `http.forbidden`; duplicate register → `identity.email_taken`; unknown errors must not leak SQL in JSON.

Minimum product cases: register/login, register without `lgpdConsent` → `identity.consent_required`, export JSON without `passwordHash`, erase then login → `identity.invalid_credentials`, SOAP 403 (`triage.forbidden`), overlapping appointment (`agenda.slot_taken`), invalid/expired/exhausted coupon, subscribe with percent coupon discounts the invoice and redeems, subscribe activates the plan (`ativa`) and a second subscribe cancels the previous one, affiliate attribution on register then subscribe projects commission, confirm-stub pays invoice and launches commission while subscription stays `ativa`, confirm-stub twice → `billing.invoice_not_open`, paciente on confirm-stub → `http.forbidden`, public plans GET, forgot-password 204, bad OTP → `identity.otp_invalid`, unverified book/subscribe → `identity.unverified`, book notifies without failing, grave triage notifies once, engine `LEVE` + “dor no peito e falta de ar” → `ALERTA_GRAVE`, SOAP stripped on `CONTINUAR_TRIAGEM`, chat after a terminal session opens a new `sessaoId`, book with an in-progress session → `triage.incomplete`, consult professional `GET /soap/{id}` 200, other professional/patient `triage.forbidden`, professional document rejects unknown kind, paciente on `/admin/audit` and `/admin/billing` → `http.forbidden`, admin export JSON without `passwordHash`, admin erase of another patient then login → `identity.invalid_credentials`, admin erase of self or `role=admin` → `http.forbidden`, login writes `identity.login`, CORS reflects an allowed origin and omits a blocked one, `APP_ENV=production` + weak `JWT_SECRET` or empty `FIELD_ENCRYPTION_KEY` fails `Load`/`Parse`, `GET /health/ready` 200 with fake pings and 503 when Redis/Postgres ping fails, login rate-limit → `http.rate_limited`, book creates teleconsultation `aberta`, titular and consult professional `POST video/join` 200 with Jitsi host in `url`, other patient/admin → `video.forbidden`, end then join → `video.ended`, cancel appointment ends the room, `GET /health` creates a span named after the chi template, `WriteError` slog includes `trace_id` when a span is on the context.

`go test ./...` **remains** the unit/HTTP-isolated contract (`make test` in `backend/`). Playwright is **opt-in**: `make test-e2e` from the repo root when `GET /health/ready` is 200 (`make db-up` + `make backend`). It runs the Vite panel (`admin/`) and the Expo web app (`mobile/`). It does not run as part of backend `make test`. First machine: `cd admin && npx playwright install` and `cd mobile && npx playwright install`. Assert `error.code` on Playwright API calls the same way as httptest. No coverage gate, no CI in this block. Native iOS/Android stays out (no Detox/Maestro). Playbook: [`arquitetura/qa/`](../qa/README.md).

Alerts: PromQL only, in [`arquitetura/ops/ALERTS.md`](../ops/ALERTS.md). Tracing: [`arquitetura/ops/TRACING.md`](../ops/TRACING.md). Data map: [`arquitetura/lgpd/DADOS.md`](../lgpd/DADOS.md).

---

## Local run

```bash
# from backend module
docker compose up -d postgres redis jaeger
go run ./cmd/http
```

Env: `DATABASE_URL`, `REDIS_URL`, `JWT_SECRET`, `APP_ENV` (`development` default;
`production` refuses short/`change-me*` JWT, empty or `*` CORS, and empty/short
`FIELD_ENCRYPTION_KEY`),
`CORS_ORIGINS` (CSV; empty in development → `*`), `CORS_ALLOW_CREDENTIALS`,
`METRICS_ENABLED` (default true), `OTEL_EXPORTER_OTLP_ENDPOINT` (empty = no-op;
local Jaeger `http://localhost:4318`), `OTEL_SERVICE_NAME`,
`FIELD_ENCRYPTION_KEY` (SHA-256 → AES-256-GCM for SOAP/history; empty in
development leaves plaintext and logs it), `ANTHROPIC_API_KEY` (triage),
`VIDEO_BASE_URL` (Jitsi host, default `https://meet.jit.si`; swap = env + restart).

Optional notify: `MAILGUN_API_KEY`, `MAILGUN_DOMAIN`, `MAILGUN_BASE_URL`, `MAIL_FROM`;
`META_WA_TOKEN`, `META_WA_PHONE_ID`, `META_WA_TEMPLATE_RESET`, `META_WA_TEMPLATE_VERIFY`,
`META_WA_TEMPLATE_LANG`; `ZAPI_BASE_URL`, `ZAPI_INSTANCE`, `ZAPI_TOKEN`, `ZAPI_CLIENT_TOKEN`.
