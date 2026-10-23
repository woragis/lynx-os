#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
run_dir="$root/.local/run"
show_process() {
  local name="$1" url="$2" pid_file="$run_dir/$1.pid"
  if [[ -f "$pid_file" ]] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
    printf 'ATIVO  %-5s PID %s — %s\n' "$name" "$(cat "$pid_file")" "$url"
  elif [[ "$name" == web ]] && curl --silent --fail "$url/" >/dev/null 2>&1; then
    printf 'ATIVO  %-5s processo preexistente — %s\n' "$name" "$url"
  elif [[ "$name" == api ]] && curl --silent --fail "$url/health/ready" >/dev/null 2>&1; then
    printf 'ATIVO  %-5s processo preexistente — %s\n' "$name" "$url"
  else
    printf 'PARADO %-5s — %s\n' "$name" "$url"
  fi
}
show_process web http://localhost:3100
show_process api http://localhost:8082
docker compose -f "$root/backend/compose.yaml" ps
printf '\nMailpit: http://localhost:8025\n'
