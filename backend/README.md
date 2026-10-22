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

As migrations SQL ficam em `migrations/` na raiz do repositório e são versionadas. A primeira reproduz o armazenamento atual de integrações, operações, conversas e auditoria; a segunda adiciona identidade administrativa, sessões revogáveis, tokens de redefinição e vínculos do Telegram. As migrations ainda precisam ser aplicadas no banco de desenvolvimento.

## Contrato SaaS Lynx v1

`internal/infrastructure/saasclient` implementa o adaptador HTTP do contrato v1 em `/api/v1/provisioning`. A camada de aplicação valida capabilities, recursos/operações e exige chave de idempotência para mutações antes de delegar ao adaptador. O cliente envia a API key somente no header `X-Lynx-API-Key`; cadastre apenas origem HTTPS, exceto `localhost`/loopback para desenvolvimento.

## Identidade e sessões administrativas

Configure `LYNX_JWT_SIGNING_KEY` com pelo menos 32 bytes aleatórios. `GET /api/v1/auth/csrf` emite o token CSRF inicial; envie-o no header `X-CSRF-Token` para `POST /login`, `POST /refresh` e mutações autenticadas. A API define access JWT de 10 minutos e refresh opaco de 14 dias em cookies HttpOnly; a sessão associada ao JWT é conferida no PostgreSQL em cada chamada para permitir revogação imediata. Refresh é rotacionado atomicamente; reutilização revoga toda a família. Cookies ficam `Secure` quando `LYNX_ENV=production`. Login aplica limite de cinco tentativas por combinação de IP/e-mail em 15 minutos e falha fechado se Redis estiver indisponível.

Depois de aplicar as migrations, crie o primeiro owner interativamente com `cd backend && set -a && . ./.env && set +a && go run ./cmd/create-admin`. A senha é lida sem eco, precisa ter de 12 a 72 bytes e é armazenada com bcrypt. O comando recusa criar outro owner. As rotas de autenticação incluem `/me`, listagem de sessões, revogação individual e revogação de todas as sessões. O JWT secret nunca deve ser rotacionado sem plano de invalidar sessões existentes.

## Cadastro e revogação de integrações

Com `LYNX_JWT_SIGNING_KEY` e `LYNX_ENCRYPTION_KEY` configurados, o backend habilita `GET/POST /api/v1/integrations`, `PUT /api/v1/integrations/{id}/api-key`, `POST /api/v1/integrations/{id}/operations` e `DELETE /api/v1/integrations/{id}`. Todas exigem sessão autenticada e CSRF para mutações. Owner/admin podem gerir integrações; owner/admin/operator podem executar operações; viewer tem leitura. Cadastro e rotação validam a API key consultando capabilities v1 do SaaS antes de persistir. As listagens nunca retornam a chave; ela é cifrada com AES-256-GCM. Operações recuperam a chave só em memória e encaminham `Idempotency-Key` ao SaaS. Revogar desativa a integração e apaga o ciphertext local. Revogue também a credencial no SaaS. Gere `LYNX_ENCRYPTION_KEY` com 32 bytes aleatórios representados por 64 caracteres hexadecimais e mantenha backup seguro: perder essa chave impede descriptografar as integrações ativas.

Exemplo de cadastro: `POST /api/v1/integrations` com `{ "name": "EasyBill", "slug": "easybill", "baseUrl": "https://easybill.example", "apiKey": "..." }`.

## Segurança e estado

API keys de SaaS e payloads/respostas de provisionamento permanecem cifrados pelo application layer. Migrations guardam somente colunas cifradas e hashes. `.env` e credenciais reais não são versionados. Não apontar o Next.js existente para este banco até implementar migração validada e cutover.
