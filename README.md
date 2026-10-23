# Lynx OS

Central de operações da Lynx para integrar os SaaS e provisionar uma empresa, o primeiro cliente e o usuário responsável. Aplicação Next.js 16 App Router, React 19, TypeScript estrito e Prisma 7 com SQLite no primeiro ambiente.

## Desenvolvimento

1. Copie `.env.example` para `.env` e configure `LYNX_GO_API_URL` e `LYNX_GO_INTERNAL_URL` com a origem local do backend Go (`http://localhost:8082`). Gere `LYNX_ENCRYPTION_KEY` com `openssl rand -hex 32` (64 caracteres hexadecimais).
2. Configure `DATABASE_URL="file:./lynx-os.db"`.
3. Execute `npm install`, `npm run db:migrate -- --name initial` e `npm run dev`.

O primeiro owner é criado interativamente pelo comando Go `go run ./cmd/create-admin`; a senha não fica em `.env`. `.env` é ignorado pelo Git.

Em **Segurança**, cada administrador pode encerrar sessões abertas, redefinir a própria senha e vincular o Telegram com um código de 8 dígitos válido por 10 minutos. O owner também tem a página **Equipe** para convidar administradores com papéis limitados, alterar papéis e desativar contas. Convites expiram em 24 horas; em desenvolvimento, a redefinição chega ao Mailpit local (`http://localhost:8025`), sem Mailgun. Tokens de sessão e de uso único são armazenados como hashes no PostgreSQL.

## Telegram

O bot agora pode rodar dentro do processo HTTP Go. Aplique a migration `000005`, configure `TELEGRAM_BOT_TOKEN`, `OPENAI_API_KEY`, `OPENAI_MODEL` e `LYNX_ENCRYPTION_KEY` em `backend/.env`, e inicie apenas `go run ./cmd/http`. Não rode simultaneamente o transporte legado (`npm run telegram:dev`), pois ambos disputariam os updates do mesmo bot. O worker usa long polling, dispensa webhook público e adquire um advisory lock PostgreSQL para manter apenas um poller ativo; conversas são cifradas no PostgreSQL.

O uso exige conta administrativa ativa vinculada pela página Segurança e pelo comando `/link CÓDIGO`. `/start` abre o menu; a IA usa a Responses API com saída JSON estruturada para preencher o rascunho incremental e não envia mutações sem confirmação. `OPENAI_MODEL` tem padrão `gpt-6-luna`. Em desenvolvimento, o nome Telegram e a chave OpenAI ficam apenas no ambiente do backend Go. O transporte Next legado permanece no repositório durante a transição e não deve ser iniciado junto com o worker novo.

## Backend Go

O backend HTTP em `backend/` segue a arquitetura modular definida em [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). A autenticação administrativa e gestão de integrações já usam Go, PostgreSQL e Redis. Parte das ações e leituras de produto ainda usa Next/Prisma e será migrada em etapas.

Para subir PostgreSQL, Redis e Mailpit, execute `docker compose -f backend/compose.yaml up -d`. Copie `backend/.env.example` para `backend/.env`, configure `LYNX_JWT_SIGNING_KEY` (`openssl rand -hex 32`) e `LYNX_ENCRYPTION_KEY`, aplique as migrations `000001` a `000005` e crie o owner com `go run ./cmd/create-admin`. Para ativar o bot integrado, preencha `TELEGRAM_BOT_TOKEN` e `OPENAI_API_KEY` no ambiente do backend. Rode `cd backend && set -a && . ./.env && set +a && go run ./cmd/http`. Em desenvolvimento, abra `http://localhost:8025` para ver os e-mails de recuperação capturados localmente. O Next encaminha `/api/v1/*` ao backend pela rewrite same-origin. Fora de desenvolvimento, configure Mailgun no `.env` do backend. A migração dos dados SQLite e das demais rotas de negócio ainda não foi feita.

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
