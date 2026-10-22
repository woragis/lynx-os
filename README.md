# Lynx OS

Central de operações da Lynx para integrar os SaaS e provisionar uma empresa, o primeiro cliente e o usuário responsável. Aplicação Next.js 16 App Router, React 19, TypeScript estrito e Prisma 7 com SQLite no primeiro ambiente.

## Desenvolvimento

1. Copie `.env.example` para `.env` e configure as variáveis locais. Gere os segredos com `openssl rand -hex 32` para `LYNX_SESSION_SECRET` e `LYNX_ENCRYPTION_KEY` (a chave de cifragem precisa ter 64 caracteres hexadecimais).
2. Configure `DATABASE_URL="file:./lynx-os.db"`.
3. Execute `npm install`, `npm run db:migrate -- --name initial` e `npm run dev`.

O primeiro usuário é configurado por `LYNX_ADMIN_EMAIL` e `LYNX_ADMIN_PASSWORD`. Nunca use os valores de exemplo em produção. `.env` é ignorado pelo Git.

## Telegram

O transporte do bot roda em um serviço dedicado em Go, em `telegram-bot/`. Ele valida o webhook do Telegram, encaminha a atualização ao endpoint interno autenticado do Next.js e envia a resposta ao chat. O Next.js segue responsável por OpenAI, conversas, allowlist e provisionamento.

Configure `TELEGRAM_BOT_TOKEN`, `LYNX_TELEGRAM_INTERNAL_TOKEN`, `LYNX_OS_INTERNAL_URL` e `TELEGRAM_ALLOWED_USER_IDS`. Gere o token interno com pelo menos 32 caracteres aleatórios; o mesmo valor precisa estar no Next.js e no processo Go. Em desenvolvimento, inicie o bot com `npm run telegram:dev`. O serviço usa long polling, então não precisa de domínio ou webhook público. O bot só aceita mensagens privadas de IDs presentes na allowlist. A IA prepara o rascunho; apenas `CONFIRMAR` autoriza o envio ao SaaS. `CANCELAR` descarta o rascunho.

O fluxo e as opções de execução do processo estão em [`telegram-bot/README.md`](telegram-bot/README.md). Em produção, publique o serviço Go atrás do HTTPS do domínio e mantenha o endpoint Next.js interno na rede privada.

O modelo padrão é `gpt-6-luna` e pode ser substituído por `OPENAI_MODEL`. A aplicação usa a Responses API e saída JSON estruturada. A chave de OpenAI fica em `OPENAI_API_KEY`, somente no servidor Next.js; o serviço Go não recebe essa chave.

## Backend Go (fundação em migração)

O novo backend HTTP começa em `backend/`, seguindo a arquitetura modular definida em [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). Ele já inicializa PostgreSQL/Redis e oferece `GET /health/live` e `GET /health/ready`; as rotas de produto continuam no Next.js até migrarem e o corte ser feito explicitamente.

Para subir as dependências locais, execute `docker compose -f backend/compose.yaml up -d`. Configure `backend/.env.example` e rode `cd backend && go run ./cmd/http`. A primeira migration PostgreSQL está em `migrations/`; ela ainda não foi aplicada nem há importação dos dados SQLite. A tela web e o transporte Telegram existentes continuam usando o backend Next.js/serviço Telegram até as próximas etapas.

## Integrações

Cadastre em **SaaS e integrações** o nome, URL da API, URL do painel e uma API key com os escopos mínimos necessários (`provisioning:read`, `provisioning:write`; `provisioning:purge` somente para purga definitiva). O cadastro valida a conexão antes de salvar. A chave é cifrada com AES-256-GCM usando `LYNX_ENCRYPTION_KEY`. **Empresas e acessos** e **Novo cadastro** usam o mesmo contrato SaaS v1 em `/api/v1/provisioning`; ele anuncia campos e operações disponíveis para cada sistema.

No EasyBill já é possível listar, abrir e editar contas, clientes e vínculos de usuário, além de hibernar, reativar e solicitar purga após o prazo. Criação individual de cliente e usuário ainda não está disponível no endpoint EasyBill: até ela ser implementada, use o bootstrap em **Novo cadastro** para criar uma nova empresa com o cliente inicial e o owner.

Por enquanto o banco é SQLite local. Em produção, use um volume persistente e uma única instância da aplicação. As chaves OpenAI, Telegram, sessão e cifragem devem vir do gerenciador de segredos do ambiente de deploy. A rotação de `LYNX_ENCRYPTION_KEY` exige recriptografar os registros antes de descartar a chave antiga.

## Testes

- `npm run test:unit`: testes unitários de funções isoladas (Vitest).
- `npm run test:integration`: testes com SQLite isolado e um SaaS HTTP local simulado; não chama Telegram, OpenAI ou serviços externos.
- `npm run test:e2e`: fluxos do painel em Chromium via Playwright. Na primeira execução, instale o navegador com `npx playwright install chromium`.
- `npm test`: executa os três níveis em sequência.

Os bancos de teste ficam em `.test/`, separados do banco de desenvolvimento e ignorados pelo Git. As credenciais E2E são descartáveis e definidas no arquivo de configuração.
