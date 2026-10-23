#!/usr/bin/env bash
set -euo pipefail
missing=0
for tool in make docker go node npm openssl psql curl; do
  if command -v "$tool" >/dev/null 2>&1; then printf 'OK   %s\n' "$tool"; else printf 'FALTA %s\n' "$tool"; missing=1; fi
done
if command -v docker >/dev/null 2>&1; then
  if docker compose version >/dev/null 2>&1; then printf 'OK   docker compose\n'; else printf 'FALTA docker compose plugin\n'; missing=1; fi
  if docker info >/dev/null 2>&1; then printf 'OK   Docker daemon\n'; else printf 'FALTA Docker daemon ativo\n'; missing=1; fi
fi
if [[ $missing -ne 0 ]]; then echo "Instale os itens ausentes e execute make doctor novamente." >&2; exit 1; fi
