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

As migrations SQL ficam em `migrations/` na raiz do repositório e são versionadas. A primeira reproduz o armazenamento atual de integrações, operações, conversas e auditoria; ainda não foi aplicada nem existe rotina de migração do SQLite.

## Contrato SaaS Lynx v1

`internal/infrastructure/saasclient` implementa o adaptador HTTP do contrato v1 em `/api/v1/provisioning`. A camada de aplicação valida capabilities, recursos/operações e exige chave de idempotência para mutações antes de delegar ao adaptador. O cliente envia a API key somente no header `X-Lynx-API-Key`; cadastre apenas origem HTTPS, exceto `localhost`/loopback para desenvolvimento.

## Cadastro e revogação de integrações

Com `LYNX_ADMIN_API_KEY` e `LYNX_ENCRYPTION_KEY` configurados, o backend habilita `GET/POST /api/v1/integrations`, `PUT /api/v1/integrations/{id}/api-key` e `DELETE /api/v1/integrations/{id}`. Todas exigem `Authorization: Bearer <LYNX_ADMIN_API_KEY>`. Cadastro e rotação validam a API key consultando capabilities v1 do SaaS antes de persistir. As listagens nunca retornam a chave; ela é cifrada com AES-256-GCM. Revogar desativa a integração e apaga o ciphertext local. Revogue também a credencial no SaaS. Gere `LYNX_ENCRYPTION_KEY` com 32 bytes aleatórios representados por 64 caracteres hexadecimais e mantenha backup seguro: perder essa chave impede descriptografar as integrações ativas.

Exemplo de cadastro: `POST /api/v1/integrations` com `{ "name": "EasyBill", "slug": "easybill", "baseUrl": "https://easybill.example", "apiKey": "..." }`. O token administrativo estático é uma ponte temporária de migração; deve ser substituído por identidade administrativa e sessões próprias antes de abrir a API para uso geral.

## Segurança e estado

API keys de SaaS e payloads/respostas de provisionamento permanecem cifrados pelo application layer. Migrations guardam somente colunas cifradas e hashes. `.env` e credenciais reais não são versionados. Não apontar o Next.js existente para este banco até implementar migração validada e cutover.
