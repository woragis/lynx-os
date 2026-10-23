#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ ! -f "$root/backend/.env" ]]; then echo "backend/.env não existe; execute make setup." >&2; exit 1; fi
if [[ $# -eq 0 ]]; then echo "Informe o comando a executar." >&2; exit 2; fi
cd "$root/backend"
set -a
source .env
set +a
exec "$@"
