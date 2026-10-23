# Estrutura do Lynx OS

O repositório separa aplicação web, backend de negócio e documentação:

    lynx-os/
    ├── backend/
    │   ├── cmd/                 executáveis Go: HTTP, bootstrap e importação
    │   ├── internal/            domínio, aplicação, infraestrutura e interfaces
    │   ├── migrations/          migrations PostgreSQL do backend
    │   ├── scripts/             operação do banco e aplicação de migrations
    │   └── legacy/              SQLite e artefatos Prisma preservados
    ├── web/
    │   ├── src/                 app router, componentes e código da interface
    │   ├── public/              assets servidos pelo Next.js
    │   ├── tests/               testes de integração e E2E
    │   └── package.json         dependências, scripts e configuração web
    ├── docs/                    arquitetura, contrato, operação e estrutura
    └── .github/                 automação de CI do repositório

## Frontend

web/ é a raiz do projeto Next.js: é nela que ficam package.json, lockfile, configurações, variáveis locais, src e public. Execute comandos Node a partir de web/. O frontend chama a API Go e não carrega credenciais SaaS no browser.

## Backend

backend/ contém o monólito modular Go, incluindo o worker Telegram no processo HTTP, migrations PostgreSQL e ferramentas de operação. legacy/ preserva a fonte SQLite e o schema/histórico Prisma antigo; esses itens não são usados pelo runtime novo. Não apague os dados legados até concluir as validações e decidir sua retenção.

## Migrations

As migrations versionadas estão em backend/migrations/; aplique-as com backend/scripts/migrate-postgres.sh a partir da raiz do repositório. Mover os arquivos não aplica nem desfaz migrations no banco. Uma mudança de schema deve continuar entrando como nova migration, sem editar arquivos já aplicados.
