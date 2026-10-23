# Operação do Lynx OS

## Inicialização e migrations

Use um ambiente explicitamente selecionado. O backend e o Next não devem compartilhar `.env` de produção. Em desenvolvimento, copie `backend/.env.example` para `backend/.env`, revise as portas e carregue esse arquivo na sessão antes de iniciar os comandos.

```sh
docker compose -f backend/compose.yaml up -d postgres redis mailpit
cd backend && set -a && . ./.env && set +a && cd ..
./backend/scripts/migrate-postgres.sh
```

O script aplica as migrations `*.up.sql` em ordem, registra o SHA-256 de cada arquivo e interrompe se uma migration já aplicada tiver sido editada. Mudanças de schema entram sempre em uma nova migration. Faça backup do banco antes de executar em qualquer ambiente com dados. O script não executa migrations `down` automaticamente.

## Segredos e rotação

- Armazene os segredos num secret manager do ambiente; injete-os no processo sem gravá-los em imagem, log, Git ou artefato CI. `backend/.env.example` contém somente valores vazios/de desenvolvimento.
- `LYNX_JWT_SIGNING_KEY`: rotacionar invalida access tokens ativos; planeje encerrar/reautenticar sessões e troque em janela controlada.
- `LYNX_ENCRYPTION_KEY`: necessária para descriptografar API keys, operações e rascunhos Telegram. O formato atual não guarda versão por chave; não a troque enquanto existirem dados cifrados. Faça primeiro uma migration de recriptografia e backup testado.
- API keys de SaaS: rotacione no SaaS, atualize no Lynx OS e revogue a anterior no SaaS. Em incidente, revogue a integração no Lynx OS e a chave remota imediatamente.
- Telegram/OpenAI/Mailgun: gere novos tokens no respectivo provedor, atualize o secret manager, reinicie/valide o serviço e revogue a credencial antiga. Em desenvolvimento, recuperação de senha usa Mailpit e não requer Mailgun.
- Redação de logs: não inclua headers de autenticação, request bodies, respostas completas do SaaS, códigos de recuperação, cookies ou prompts com dados pessoais.

## Rede e probes

- Publique API e Next atrás de proxy com TLS; configure `LYNX_ENV=production` para cookies `Secure`. Não exponha PostgreSQL, Redis, Mailpit nem portas administrativas à internet.
- O navegador acessa o Next; o servidor Next fala com o Go. Não habilitamos CORS amplo. Preserve validação CSRF e cookies same-site.
- `GET /health/live` verifica o processo. `GET /health/ready` verifica PostgreSQL e Redis e retorna `503` se algum deles falhar. As probes não garantem acesso ao Telegram, OpenAI ou a cada SaaS; monitore falhas do worker e operações pela telemetria/logs do host.
- Timeout HTTP, limite de corpo e rate limiting precisam ser aplicados também no proxy de borda antes de expor a API publicamente. Métricas Prometheus ainda não estão implementadas.
- Execute uma única instância ativa de polling Telegram por banco. O advisory lock PostgreSQL impede pollers simultâneos entre réplicas.

## Backup, restauração e recuperação

Faça backup consistente PostgreSQL com `pg_dump --format=custom`; cifre e armazene fora do host. Inclua no mesmo plano a cópia segura de `LYNX_ENCRYPTION_KEY`, `LYNX_JWT_SIGNING_KEY` e secrets de provedores, com acesso restrito. Redis guarda limitadores de login; perder Redis reinicia suas janelas de rate limit, mas não remove a fonte de verdade de contas/sessões.

Restaure primeiro em banco isolado com `pg_restore`, injete as mesmas chaves de cifragem e valide login, integração, descriptografia e uma operação SaaS segura de leitura antes de promover. Nunca teste restore sobre o banco ativo. Mantenha um backup pré-migration para rollback; prefira migration corretiva para frente, pois `down` pode apagar dados.

## Incidentes externos e corte

- Se um SaaS estiver indisponível, operações falhas permanecem auditadas e podem ser repetidas com a mesma chave de idempotência. Confirme no SaaS o resultado remoto antes de tentar uma ação nova.
- Se Telegram/OpenAI cair, o painel HTTP permanece utilizável; o worker volta a tentar polling e registra erro sem registrar tokens/conteúdo sensível.
- Antes do cutover: congelar escritas no sistema antigo, gerar backup verificável, executar importador em dry-run, importar em destino isolado, reconciliar contagens e testar contas/integrações com acesso restrito.
- O importador e as migrations foram verificados em PostgreSQL descartável. Ainda falta validar o banco PostgreSQL real de desenvolvimento e EasyBill staging, ensaiar backup/restore e observar a pipeline CI no remoto. Não existe deploy/cutover de produção aprovado ou executado.
