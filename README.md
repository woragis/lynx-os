# Lynx OS

Central de operações da Lynx para integrar os SaaS e provisionar empresas, clientes e usuários. O Next.js 16 é a interface; a API Go é responsável por autenticação, regras, persistência PostgreSQL, auditoria, integrações e bot Telegram.

## Desenvolvimento

1. Copie `.env.example` para `.env` e configure `LYNX_GO_API_URL` e `LYNX_GO_INTERNAL_URL` com a origem local da API Go (`http://localhost:8082`).
2. Execute `npm install` e `npm run dev`.

O primeiro owner é criado interativamente pelo comando Go `go run ./cmd/create-admin`; a senha não fica em `.env`. `.env` é ignorado pelo Git.

Em **Segurança**, cada administrador pode encerrar sessões abertas, redefinir a própria senha e vincular o Telegram com um código de 8 dígitos válido por 10 minutos. O owner também tem a página **Equipe** para convidar administradores com papéis limitados, alterar papéis e desativar contas. Convites expiram em 24 horas; em desenvolvimento, a redefinição chega ao Mailpit local (`http://localhost:8025`), sem Mailgun. Tokens de sessão e de uso único são armazenados como hashes no PostgreSQL.

## Telegram

O polling e a lógica do bot estão dentro do processo HTTP Go. Configure `TELEGRAM_BOT_TOKEN`, `OPENAI_API_KEY`, `OPENAI_MODEL` e `LYNX_ENCRYPTION_KEY` em `backend/.env`. O worker usa long polling, dispensa webhook público e adquire um advisory lock PostgreSQL para manter apenas um poller ativo; conversas são cifradas no PostgreSQL.

O uso exige conta administrativa ativa vinculada pela página Segurança e pelo comando `/link CÓDIGO`. `/start` abre o menu; a IA usa a Responses API com saída JSON estruturada para preencher o rascunho incremental e não envia mutações sem confirmação. `OPENAI_MODEL` tem padrão `gpt-6-luna`. As chaves do Telegram e OpenAI ficam somente no ambiente do backend Go.

## Backend Go

O backend HTTP em `backend/` segue a arquitetura modular definida em [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). Autenticação, integrações e operações de SaaS usam Go, PostgreSQL e Redis. O Next faz chamadas server-side à API Go e encaminha as rotas de autenticação same-origin.

Para subir PostgreSQL, Redis e Mailpit, execute `docker compose -f backend/compose.yaml up -d`. Copie `backend/.env.example` para `backend/.env`, configure `LYNX_JWT_SIGNING_KEY` (`openssl rand -hex 32`) e `LYNX_ENCRYPTION_KEY`, aplique as migrations `000001` a `000006` e crie o owner com `go run ./cmd/create-admin`. Para ativar o bot integrado, preencha `TELEGRAM_BOT_TOKEN` e `OPENAI_API_KEY` no ambiente do backend. Rode `cd backend && set -a && . ./.env && set +a && go run ./cmd/http`. Em desenvolvimento, abra `http://localhost:8025` para ver os e-mails de recuperação capturados localmente. O Next encaminha `/api/v1/*` ao backend pela rewrite same-origin. Fora de desenvolvimento, configure Mailgun no `.env` do backend. A migração dos registros já existentes no SQLite está descrita e será conduzida pelo item LNX-008 antes do descarte do arquivo.

## Integrações

Cadastre em **SaaS e integrações** o nome, URL da API, URL do painel e uma API key com os escopos mínimos necessários (`provisioning:read`, `provisioning:write`; `provisioning:purge` somente para purga definitiva). O cadastro valida a conexão antes de salvar. A chave é cifrada com AES-256-GCM usando `LYNX_ENCRYPTION_KEY`. **Empresas e acessos** e **Novo cadastro** usam o mesmo contrato SaaS v1 em `/api/v1/provisioning`; ele anuncia campos e operações disponíveis para cada sistema.

No EasyBill já é possível listar, abrir e editar contas, clientes e vínculos de usuário, além de hibernar, reativar e solicitar purga após o prazo. Criação individual de cliente e usuário ainda não está disponível no endpoint EasyBill: até ela ser implementada, use o bootstrap em **Novo cadastro** para criar uma nova empresa com o cliente inicial e o owner.

Os registros atuais do SQLite foram inventariados e permanecem preservados como fonte de migração; o Next não os consulta mais em runtime. As chaves OpenAI, Telegram, `LYNX_JWT_SIGNING_KEY` e `LYNX_ENCRYPTION_KEY` devem vir do gerenciador de segredos do ambiente de deploy. A rotação do JWT encerra sessões; a rotação da chave de cifragem exige recriptografar registros antes de descartar a chave antiga.

## Testes

- `npm run test:unit`: testes unitários do backend Go.
- `npm run test:integration`: testes do adaptador Next para a API Go com transporte HTTP mockado; não chama SaaS, Telegram ou OpenAI externos.
- `npm run test:e2e`: fluxos do painel em Chromium via Playwright. Na primeira execução, instale o navegador com `npx playwright install chromium`.
- `npm test`: executa os três níveis em sequência.

Os testes automatizados não dependem do banco SQLite original nem de credenciais externas. Os E2E usam uma API de autenticação local simulada e Chromium.
