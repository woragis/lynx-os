# Desenvolvimento local com Make

O Makefile na raiz centraliza o ambiente de desenvolvimento do Lynx OS. Execute os comandos na raiz do repositório; não é necessário informar portas nem caminhos.

## Primeiro uso

```sh
make setup
make owner
make up
```

`make setup` cria `backend/.env` e `web/.env` a partir dos exemplos quando ainda não existem. Na primeira criação do ambiente Go, gera as chaves locais de sessão e de cifragem. Também verifica as ferramentas e instala as dependências do frontend quando necessário.

Arquivos `.env` existentes são pontos fixos: `make setup` e `make up` os preservam sem copiar, recriar ou gerar backups automáticos. Se um deles ainda não existir, apenas esse arquivo ausente é criado a partir do exemplo. Para criar um backup manual dos arquivos atuais, execute `make env-backup`. Os `.env` e backups são locais, têm permissão restrita e não entram no Git. Tokens de Telegram e OpenAI nunca são gerados pelo setup.

`make owner` aplica as migrations e abre o fluxo interativo para criar o primeiro owner. Se o banco ainda não tiver um owner, faça esse passo antes de iniciar a aplicação.

## Comandos do dia a dia

| Comando | O que faz |
| --- | --- |
| `make up` | Sobe PostgreSQL, Redis e Mailpit, aplica migrations e inicia API e frontend em background. |
| `make down` | Encerra os processos iniciados pelo Make e os containers; preserva os volumes do banco. |
| `make status` | Mostra PIDs, containers e URLs locais. |
| `make logs` | Acompanha os arquivos `.local/logs/api.log` e `web.log`; use `Ctrl+C` para sair. |
| `make doctor` | Confere Make, Docker/Compose, Go, Node/npm, OpenSSL, `psql` e `curl`. |
| `make migrate` | Sobe a infraestrutura e aplica migrations sem iniciar as aplicações. |
| `make env-backup` | Faz backup dos `.env` existentes sem modificá-los. |
| `make infra-up` / `make infra-down` | Controla apenas os containers locais. |
| `make api` / `make web` | Executa uma aplicação em primeiro plano para desenvolvimento isolado. |

URLs padrão:

- Aplicação: <http://localhost:3100>
- API Go: <http://localhost:8082>
- Saúde da API: <http://localhost:8082/health/ready>
- Mailpit: <http://localhost:8025>
- PostgreSQL: `localhost:5453`
- Redis: `localhost:6382`

`make up` reutiliza uma API ou frontend Lynx que já esteja respondendo nas portas padrão, evitando iniciar cópias que disputem a mesma porta. `make down` encerra os processos registrados pelo Make e não encerra processos preexistentes que ele não iniciou. Também não usa `docker compose down -v`; o banco permanece no volume `lynx-os-dev-pgdata`. Para apagar dados, remova esse volume deliberadamente fora do fluxo normal de encerramento.

## Telegram e OpenAI

O worker do Telegram executa dentro do processo da API Go. Para ativá-lo, configure `TELEGRAM_BOT_TOKEN` em `backend/.env`. Para habilitar interpretação de mensagens naturais, configure também `OPENAI_API_KEY`. Depois, reinicie com `make up` ou `make down` e `make up`. Sem esses valores, API e frontend continuam funcionando e o bot permanece desativado. O comando `/start` abre um menu novo e remove o menu anterior quando ainda existe; os botões internos atualizam o menu atual.

## Resolução de problemas

- Se um serviço não responder durante `make up`, consulte `make status` e `make logs`.
- Se Docker estiver parado, inicie o daemon e execute `make up` novamente.
- Se as variáveis já configuradas precisarem ser revisadas, edite os arquivos `.env` diretamente; `make setup` e `make up` preservam valores existentes e não geram backups automáticos.
- Para restaurar um backup, pare os serviços, copie o arquivo desejado de `.local/backups/env/` de volta ao caminho correspondente e restrinja as permissões (`chmod 600`). Não substitua uma chave de cifragem se já houver dados cifrados no banco.
- `make down` para os processos pelos PIDs registrados em `.local/run/`; se um PID tiver sido reutilizado por outro programa, o script verifica a linha de comando antes de enviar sinal.
