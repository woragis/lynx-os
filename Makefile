SHELL := /bin/bash
.DEFAULT_GOAL := help
ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
SCRIPTS := $(ROOT)/scripts

.PHONY: help setup doctor env-backup up down start stop status logs infra-up infra-down migrate owner api web

help:
	@printf '%s\n' 'Lynx OS — comandos locais' '' '  make setup       Prepara .env e dependências' '  make up          Inicia banco, Redis, Mailpit, API e frontend' '  make down        Para tudo sem apagar volumes do banco' '  make status      Exibe estado dos serviços e URLs locais' '  make logs        Acompanha logs da API e do frontend' '  make doctor      Verifica ferramentas necessárias' '  make migrate     Aplica migrations PostgreSQL' '  make owner       Cria interativamente o primeiro owner' '  make api         Executa somente a API em primeiro plano' '  make web         Executa somente o frontend em primeiro plano' '  make infra-up    Inicia apenas PostgreSQL, Redis e Mailpit' '  make infra-down  Para apenas a infraestrutura'

setup:
	@"$(SCRIPTS)/doctor.sh"
	@"$(SCRIPTS)/init-env.sh"
	@if [[ ! -x "$(ROOT)/web/node_modules/.bin/next" ]]; then cd "$(ROOT)/web" && npm ci; fi

doctor:
	@"$(SCRIPTS)/doctor.sh"

env-backup:
	@"$(SCRIPTS)/init-env.sh" --backup-only

up: setup
	@"$(SCRIPTS)/dev-up.sh"
start: up

down:
	@"$(SCRIPTS)/dev-down.sh"
stop: down

status:
	@"$(SCRIPTS)/dev-status.sh"

logs:
	@mkdir -p "$(ROOT)/.local/logs"
	@touch "$(ROOT)/.local/logs/api.log" "$(ROOT)/.local/logs/web.log"
	@tail -n 100 -f "$(ROOT)/.local/logs/api.log" "$(ROOT)/.local/logs/web.log"

infra-up:
	@docker compose -f "$(ROOT)/backend/compose.yaml" up -d --wait
infra-down:
	@docker compose -f "$(ROOT)/backend/compose.yaml" down

migrate: setup infra-up
	@"$(SCRIPTS)/with-backend-env.sh" "$(ROOT)/backend/scripts/migrate-postgres.sh"
owner: setup migrate
	@"$(SCRIPTS)/with-backend-env.sh" go run ./cmd/create-admin
api: setup infra-up migrate
	@"$(SCRIPTS)/with-backend-env.sh" go run ./cmd/http
web: setup
	@cd "$(ROOT)/web" && npm run dev -- --hostname 127.0.0.1 --port 3100
