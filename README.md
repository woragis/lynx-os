# Lynx OS

Central de operações da Lynx para integrar os SaaS e provisionar uma empresa, o primeiro cliente e o usuário responsável. Aplicação Next.js 16 App Router, React 19, TypeScript estrito e Prisma 7 com SQLite no primeiro ambiente.

## Desenvolvimento

1. Copie `.env.example` para `.env` e configure `LYNX_GO_API_URL` e `LYNX_GO_INTERNAL_URL` com a origem local do backend Go (`http://localhost:8082`). Gere `LYNX_ENCRYPTION_KEY` com `openssl rand -hex 32` (64 caracteres hexadecimais).
2. Configure `DATABASE_URL="file:./lynx-os.db"`.
3. Execute `npm install`, `npm run db:migrate -- --name initial` e `npm run dev`.

O primeiro owner é criado interativamente pelo comando Go `go run ./cmd/create-admin`; a senha não fica em `.env`. `.env` é ignorado pelo Git.

## Telegram

O transporte do bot roda em um serviço dedicado em Go, em `telegram-bot/`. Ele valida o webhook do Telegram, encaminha a atualização ao endpoint interno autenticado do Next.js e envia a resposta ao chat. O Next.js segue responsável por OpenAI, conversas, allowlist e provisionamento.

Configure `TELEGRAM_BOT_TOKEN`, `LYNX_TELEGRAM_INTERNAL_TOKEN`, `LYNX_OS_INTERNAL_URL` e `TELEGRAM_ALLOWED_USER_IDS`. Gere o token interno com pelo menos 32 caracteres aleatórios; o mesmo valor precisa estar no Next.js e no processo Go. Em desenvolvimento, inicie o bot com `npm run telegram:dev`. O serviço usa long polling, então não precisa de domínio ou webhook público. O bot só aceita mensagens privadas de IDs presentes na allowlist. A IA prepara o rascunho; apenas `CONFIRMAR` autoriza o envio ao SaaS. `CANCELAR` descarta o rascunho.

O fluxo e as opções de execução do processo estão em [`telegram-bot/README.md`](telegram-bot/README.md). Em produção, publique o serviço Go atrás do HTTPS do domínio e mantenha o endpoint Next.js interno na rede privada.

O modelo padrão é `gpt-6-luna` e pode ser substituído por `OPENAI_MODEL`. A aplicação usa a Responses API e saída JSON estruturada. A chave de OpenAI fica em `OPENAI_API_KEY`, somente no servidor Next.js; o serviço Go não recebe essa chave.

## Backend Go

O backend HTTP em `backend/` segue a arquitetura modular definida em [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). A autenticação administrativa e gestão de integrações já usam Go, PostgreSQL e Redis. Parte das ações e leituras de produto ainda usa Next/Prisma e será migrada em etapas.

Para subir PostgreSQL e Redis, execute `docker compose -f backend/compose.yaml up -d`. Copie `backend/.env.example` para `backend/.env`, configure `LYNX_JWT_SIGNING_KEY` (`openssl rand -hex 32`) e `LYNX_ENCRYPTION_KEY`, aplique as migrations `000001` e `000002` e crie o owner com `go run ./cmd/create-admin`. Rode `cd backend && set -a && . ./.env && set +a && go run ./cmd/http`. O Next encaminha `/api/v1/*` ao backend pela rewrite same-origin. A migração dos dados SQLite e das demais rotas de negócio ainda não foi feita.

## Integrações

Cadastre em **SaaS e integrações** o nome, URL da API, URL do painel e uma API key com os escopos mínimos necessários (`provisioning:read`, `provisioning:write`; `provisioning:purge` somente para purga definitiva). O cadastro valida a conexão antes de salvar. A chave é cifrada com AES-256-GCM usando `LYNX_ENCRYPTION_KEY`. **Empresas e acessos** e **Novo cadastro** usam o mesmo contrato SaaS v1 em `/api/v1/provisioning`; ele anuncia campos e operações disponíveis para cada sistema.

No EasyBill já é possível listar, abrir e editar contas, clientes e vínculos de usuário, além de hibernar, reativar e solicitar purga após o prazo. Criação individual de cliente e usuário ainda não está disponível no endpoint EasyBill: até ela ser implementada, use o bootstrap em **Novo cadastro** para criar uma nova empresa com o cliente inicial e o owner.

Por enquanto, os dados de produto no Next continuam em SQLite local. Em produção, use PostgreSQL para o backend Go e planeje a migração dos dados antes do corte. As chaves OpenAI, Telegram, `LYNX_JWT_SIGNING_KEY` e `LYNX_ENCRYPTION_KEY` devem vir do gerenciador de segredos do ambiente de deploy. A rotação do JWT encerra sessões; a rotação da chave de cifragem exige recriptografar registros antes de descartar a chave antiga.

## Testes

- `npm run test:unit`: testes unitários de funções isoladas (Vitest).
- `npm run test:integration`: testes com SQLite isolado e um SaaS HTTP local simulado; não chama Telegram, OpenAI ou serviços externos.
- `npm run test:e2e`: fluxos do painel em Chromium via Playwright. Na primeira execução, instale o navegador com `npx playwright install chromium`.
- `npm test`: executa os três níveis em sequência.

Os bancos de teste ficam em `.test/`, separados do banco de desenvolvimento e ignorados pelo Git. As credenciais E2E são descartáveis e definidas no arquivo de configuração.
