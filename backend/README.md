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

`internal/infrastructure/saasclient` implementa o adaptador HTTP do contrato v1 em `/api/v1/provisioning`. A camada de aplicação valida capabilities, recursos/operações e exige chave de idempotência para mutações antes de delegar ao adaptador. O cliente envia a API key somente no header `X-Lynx-API-Key`; cadastre apenas origem HTTPS, exceto `localhost`/loopback para desenvolvimento. O cliente ainda não está ligado a um caso de uso persistido nem exposto por uma rota administrativa.

## Segurança e estado

API keys de SaaS e payloads/respostas de provisionamento permanecem cifrados pelo application layer. Migrations guardam somente colunas cifradas e hashes. `.env` e credenciais reais não são versionados. Não apontar o Next.js existente para este banco até implementar migração validada e cutover.
