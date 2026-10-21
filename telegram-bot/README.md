# Lynx Telegram Bot (Go)

Este processo usa long polling para buscar atualizações diretamente do Telegram, encaminha cada mensagem ao Next.js e envia a resposta devolvida pelo backend. Não requer domínio público nem webhook.

O Next.js continua dono da lógica de produto: autorização por Telegram ID, OpenAI, estado da conversa, confirmação e chamadas aos SaaS. O contrato interno é `POST /api/internal/telegram/update`, autenticado com `Authorization: Bearer <LYNX_TELEGRAM_INTERNAL_TOKEN>`, que responde `{ "reply": string | null }`.

## Desenvolvimento

Configure no `.env` da raiz:

- `TELEGRAM_BOT_TOKEN`: token atual do bot, usado só pelo processo Go e pelo script de webhook;
- `LYNX_TELEGRAM_INTERNAL_TOKEN`: segredo aleatório de pelo menos 32 caracteres, igual no Go e no Next.js;
- `LYNX_OS_INTERNAL_URL`: URL privada do Next.js (padrão: `http://localhost:3000`);
- `TELEGRAM_ALLOWED_USER_IDS`: IDs liberados, conferidos pelo backend Next.js.

Com Go 1.24 ou superior instalado:

```bash
npm run telegram:dev
```

O serviço mantém polling ativo e escuta a porta `8081` por padrão. `GET /healthz` verifica disponibilidade. Na inicialização, remove eventual webhook cadastrado para que o long polling seja aceito pelo Telegram.

Também há um `Dockerfile` para gerar a imagem independente do bot. O serviço falha no boot se as credenciais e a URL interna obrigatórias não estiverem configuradas.

## Segurança e operação

- O token do bot e a chave OpenAI não são enviados entre os serviços: Go chama Telegram e Next chama OpenAI.
- Go recebe as atualizações por long polling e autentica a chamada interna com bearer token.
- Atualizações têm limite de 64 KB; mensagens, tokens e dados pessoais não entram nos logs.
- Se o Next.js não aceitar a atualização, Go devolve erro para permitir retry. Se o Next processou e o envio da resposta ao Telegram falhar, Go confirma o webhook para evitar executar o comando duas vezes.
