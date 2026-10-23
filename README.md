# Lynx OS

Central de operações da Lynx para integrar SaaS e administrar empresas/contas, clientes e usuários. O frontend Next.js fica em <code>web/</code>; o backend Go, suas migrations PostgreSQL e ferramentas ficam em <code>backend/</code>. A arquitetura Go segue <code>docs/ARCHITECTURE.md</code>.

## Estrutura

- <code>web/</code>: aplicação Next.js, assets públicos, testes e configuração Node.
- <code>backend/</code>: API Go, worker Telegram integrado, migrations, scripts e arquivos legados preservados.
- <code>docs/</code>: arquitetura, contrato SaaS v1 e operação.

## Início rápido

Use o Makefile da raiz para preparar e controlar o ambiente local. No primeiro uso, execute <code>make setup</code>, crie o owner com <code>make owner</code> e inicie os serviços com <code>make up</code>. Acesse <code>http://localhost:3100</code>; Mailpit fica em <code>http://localhost:8025</code>. Para encerrar sem apagar os dados do banco, execute <code>make down</code>. Consulte <code>docs/make.md</code> para todos os comandos e detalhes dos arquivos de ambiente.

## Desenvolvimento web

    cd web
    cp .env.example .env
    npm install
    npm run dev

Configure <code>LYNX_GO_API_URL</code> e <code>LYNX_GO_INTERNAL_URL</code> com a origem local do backend (<code>http://localhost:8082</code>). O primeiro owner é criado interativamente pelo comando Go <code>go run ./cmd/create-admin</code>; a senha não fica no arquivo de ambiente.

Em **Segurança**, cada administrador pode encerrar sessões, redefinir a própria senha e vincular Telegram com código de 8 dígitos válido por 10 minutos. O owner pode convidar administradores, alterar papéis e desativar contas. Convites expiram em 24 horas; em desenvolvimento, a recuperação chega ao Mailpit (<code>http://localhost:8025</code>). Tokens de sessão e de uso único são guardados como hashes no PostgreSQL.

## Backend Go e Telegram

O polling e a lógica do bot estão dentro do processo HTTP Go. Configure <code>TELEGRAM_BOT_TOKEN</code> e <code>LYNX_ENCRYPTION_KEY</code> em <code>backend/.env</code>; <code>OPENAI_API_KEY</code> e <code>OPENAI_MODEL</code> habilitam coleta inteligente. Sem a chave OpenAI, <code>/start</code>, menus e vínculo continuam disponíveis, mas a IA não organiza mensagens naturais. O worker usa long polling e lock advisory PostgreSQL para manter um poller ativo; conversas são cifradas.

Suba PostgreSQL, Redis e Mailpit pela raiz do repositório:

    make infra-up

Configure <code>backend/.env</code> com <code>LYNX_JWT_SIGNING_KEY</code> e <code>LYNX_ENCRYPTION_KEY</code>, aplique migrations com <code>./backend/scripts/migrate-postgres.sh</code> e crie o owner:

    cd backend
    set -a && . ./.env && set +a
    go run ./cmd/create-admin
    go run ./cmd/http

Para ativar o bot, defina <code>TELEGRAM_BOT_TOKEN</code>; a coleta por IA também exige <code>OPENAI_API_KEY</code>. Em desenvolvimento, consulte e-mails de recuperação no Mailpit local.

## Migração do SQLite legado

O arquivo preservado está em <code>backend/legacy/lynx-os.db</code>. O importador valida integridade e cifra antes de importar; a conversa antiga sem vínculo administrativo confiável fica arquivada e cifrada. Mantenha o backup independente até encerrar a validação de negócio.

Com migrations aplicadas e variáveis do backend carregadas, rode o dry-run a partir de <code>backend/</code>:

    cd backend
    set -a && . ./.env && set +a
    LYNX_IMPORT_DATABASE_URL="$DATABASE_URL" go run ./cmd/import-sqlite --source legacy/lynx-os.db

Confira contagens antes de acrescentar <code>--apply</code>. O destino deve estar vazio nas tabelas importadas; a aplicação é transacional e não apaga a fonte SQLite.

## Integrações

Cadastre em **SaaS e integrações** o nome, URL da API, URL do painel e API key com escopos mínimos (<code>provisioning:read</code>, <code>provisioning:write</code>; <code>provisioning:purge</code> só para purga definitiva). A chave é cifrada com AES-256-GCM. **Empresas e acessos** e **Novo cadastro** usam o contrato SaaS v1 em <code>/api/v1/provisioning</code>.

No EasyBill é possível listar, abrir e editar contas, clientes e vínculos de usuário, além de hibernar, reativar e solicitar purga após retenção. Criação isolada de cliente/usuário ainda não está disponível; use o bootstrap de **Novo cadastro** para criar a empresa com cliente inicial e owner.

O Next não consulta SQLite em runtime. Em deploy, injete chaves OpenAI, Telegram, JWT e cifragem por secret manager. Rotacionar a chave de cifragem exige recriptografar os dados antes de descartar a anterior.

## Testes

Execute os comandos Node a partir de <code>web/</code>:

- <code>npm run test:unit</code>: testes unitários Go.
- <code>npm run test:integration</code>: adaptador Next para API Go com transporte HTTP simulado.
- <code>npm run test:e2e</code>: painel em Chromium via Playwright.
- <code>npm test</code>: executa os três níveis.

Os testes automatizados não dependem do SQLite original nem de credenciais externas. Consulte <code>docs/operacao.md</code> para backups, restore, segurança e cutover.
