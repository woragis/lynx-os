# Lynx OS

Central de operações da Lynx para integrar os SaaS e provisionar uma empresa, o primeiro cliente e o usuário responsável. Aplicação Next.js 16 App Router, React 19, TypeScript estrito e Prisma 7 com SQLite no primeiro ambiente.

## Desenvolvimento

1. Copie `.env.example` para `.env` e configure as variáveis locais. Gere os segredos com `openssl rand -hex 32` para `LYNX_SESSION_SECRET` e `LYNX_ENCRYPTION_KEY` (a chave de cifragem precisa ter 64 caracteres hexadecimais).
2. Configure `DATABASE_URL="file:./lynx-os.db"`.
3. Execute `npm install`, `npm run db:migrate -- --name initial` e `npm run dev`.

O primeiro usuário é configurado por `LYNX_ADMIN_EMAIL` e `LYNX_ADMIN_PASSWORD`. Nunca use os valores de exemplo em produção. `.env` é ignorado pelo Git.

## Telegram

Configure `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_WEBHOOK_URL` e `TELEGRAM_ALLOWED_USER_IDS`. Cadastre o webhook depois do deploy executando `npm run telegram:set-webhook`. O bot só aceita mensagens privadas de IDs presentes na allowlist. A IA prepara o rascunho; apenas `CONFIRMAR` autoriza o envio ao SaaS. `CANCELAR` descarta o rascunho.

O modelo padrão é `gpt-6-luna` e pode ser substituído por `OPENAI_MODEL`. A aplicação usa a Responses API e saída JSON estruturada. A chave de OpenAI fica em `OPENAI_API_KEY`, somente no servidor.

## Integrações

Cadastre no painel o nome e domínio base do SaaS e uma API key com somente os escopos `provisioning:read` e `provisioning:write`. A chave é cifrada com AES-256-GCM usando `LYNX_ENCRYPTION_KEY`. O painel consulta `GET /api/v1/provisioning`, confere `contractVersion: "1"` e então chama `POST /api/v1/provisioning` com `Idempotency-Key`.

Por enquanto o banco é SQLite local. Em produção, use um volume persistente e uma única instância da aplicação. As chaves OpenAI, Telegram, sessão e cifragem devem vir do gerenciador de segredos do ambiente de deploy. A rotação de `LYNX_ENCRYPTION_KEY` exige recriptografar os registros antes de descartar a chave antiga.

## Testes

- `npm run test:unit`: testes unitários de funções isoladas (Vitest).
- `npm run test:integration`: testes com SQLite isolado e um SaaS HTTP local simulado; não chama Telegram, OpenAI ou serviços externos.
- `npm run test:e2e`: fluxos do painel em Chromium via Playwright. Na primeira execução, instale o navegador com `npx playwright install chromium`.
- `npm test`: executa os três níveis em sequência.

Os bancos de teste ficam em `.test/`, separados do banco de desenvolvimento e ignorados pelo Git. As credenciais E2E são descartáveis e definidas no arquivo de configuração.
