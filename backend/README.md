# Lynx OS Go backend

Monólito modular com composição em `cmd/http`, camadas domain/application/infrastructure/interfaces e PostgreSQL + Redis. A API atual é a base de migração; o Next.js ainda é a aplicação em uso até as rotas de negócio migrarem e o corte ser planejado.

## Desenvolvimento

```sh
docker compose -f backend/compose.yaml up -d
cp backend/.env.example backend/.env
cd backend && set -a && . ./.env && set +a && go run ./cmd/http
```

- `GET /health/live`: processo HTTP ativo.
- `GET /health/ready`: verifica PostgreSQL e Redis e responde `503` se qualquer dependência não estiver pronta.

As migrations SQL ficam em `migrations/` na raiz do repositório e são versionadas. Aplique-as em ordem com `./scripts/migrate-postgres.sh`; o script registra checksums e detecta alterações retroativas. O guia de credenciais, backup/restore, health probes e cutover está em `docs/operacao.md`.

## Contrato SaaS Lynx v1

`internal/infrastructure/saasclient` implementa o adaptador HTTP do contrato v1 em `/api/v1/provisioning`. A camada de aplicação valida capabilities, recursos/operações e exige chave de idempotência para mutações antes de delegar ao adaptador. O cliente envia a API key somente no header `X-Lynx-API-Key`; cadastre apenas origem HTTPS, exceto `localhost`/loopback para desenvolvimento.

## Identidade e sessões administrativas

Configure `LYNX_JWT_SIGNING_KEY` com pelo menos 32 bytes aleatórios. `GET /api/v1/auth/csrf` emite o token CSRF inicial; envie-o no header `X-CSRF-Token` para `POST /login`, `POST /refresh` e mutações autenticadas. A API define access JWT de 10 minutos e refresh opaco de 14 dias em cookies HttpOnly; a sessão associada ao JWT é conferida no PostgreSQL em cada chamada para permitir revogação imediata. Refresh é rotacionado atomicamente; reutilização revoga toda a família. Cookies ficam `Secure` quando `LYNX_ENV=production`. Login aplica limite de cinco tentativas por combinação de IP/e-mail em 15 minutos e falha fechado se Redis estiver indisponível.

Depois de aplicar as migrations, crie o primeiro owner interativamente com `cd backend && set -a && . ./.env && set +a && go run ./cmd/create-admin`. A senha é lida sem eco, precisa ter de 12 a 72 bytes e é armazenada com bcrypt. O comando recusa criar outro owner. A API oferece login/logout, sessões revogáveis, redefinição de senha de uso único e convites de 24 horas. Em `LYNX_ENV=dev` ou `development`, a recuperação envia por SMTP ao Mailpit local; não precisa de Mailgun. Suba o serviço pelo compose e abra `http://localhost:8025` para ler a mensagem e copiar o link. O Mailpit captura a mensagem e não a entrega ao endereço real. Fora de desenvolvimento, o backend exige `MAILGUN_API_KEY`, `MAILGUN_DOMAIN`, `MAILGUN_FROM` e `LYNX_PUBLIC_URL`. Somente o owner pode convidar e alterar administradores. Os papéis `admin`, `operator` e `viewer` têm permissões distintas para integrações. O JWT secret nunca deve ser rotacionado sem plano de invalidar sessões existentes.

O owner pode gerar um código Telegram de uso único, válido por 10 minutos, em `POST /api/v1/auth/telegram/link-code`. O bot de polling roda dentro do processo Go quando `TELEGRAM_BOT_TOKEN` está configurado; requer vínculo de administrador e usa PostgreSQL para conversa, deduplicação e lock de instância única. `OPENAI_API_KEY` ativa a coleta inteligente em linguagem natural. Sem essa chave, `/start`, menus e vínculo continuam disponíveis; o backend registra um aviso e não tenta enviar dados à OpenAI. Para habilitar IA, configure a chave no ambiente do backend e reinicie o processo.

## Cadastro e revogação de integrações

Com `LYNX_JWT_SIGNING_KEY` e `LYNX_ENCRYPTION_KEY` configurados, o backend habilita `GET/POST /api/v1/integrations`, `GET /api/v1/integrations/{id}/capabilities`, `PUT /api/v1/integrations/{id}/api-key`, `POST /api/v1/integrations/{id}/operations` e `DELETE /api/v1/integrations/{id}`. Todas exigem sessão autenticada e CSRF para mutações. Owner/admin podem gerir integrações; owner/admin/operator podem executar operações; viewer tem leitura. Cadastro e rotação validam a API key consultando capabilities v1 do SaaS antes de persistir. As listagens nunca retornam a chave; ela é cifrada com AES-256-GCM. Operações recuperam a chave só em memória e encaminham `Idempotency-Key` ao SaaS. `operations` valida as capabilities publicadas antes de encaminhar `resource.list`, `resource.get`, `resource.update`, `resource.deactivate`, `resource.reactivate`, `resource.purge` ou `bootstrap`; o SaaS continua responsável por validar escopo da conta e regras de negócio. Revogar desativa a integração e apaga o ciphertext local. Revogue também a credencial no SaaS. Gere `LYNX_ENCRYPTION_KEY` com 32 bytes aleatórios representados por 64 caracteres hexadecimais e mantenha backup seguro: perder essa chave impede descriptografar as integrações ativas.

Exemplo de cadastro: `POST /api/v1/integrations` com `{ "name": "EasyBill", "slug": "easybill", "baseUrl": "https://easybill.example", "apiKey": "..." }`.

## Segurança e estado

API keys de SaaS e payloads/respostas de provisionamento permanecem cifrados pelo application layer. Cada operação persiste hash do comando, payload cifrado, estado e resposta cifrada com idempotência única por integração; uma repetição retorna a resposta original, uma chave com outro comando retorna conflito, e operações pendentes antigas podem ser reconciliadas usando a idempotência do próprio SaaS. A auditoria guarda ator, integração, ação, request ID e resultado sem payload. `.env` e credenciais reais não são versionados. Não apontar o Next.js existente para este banco até implementar migração validada e cutover.
