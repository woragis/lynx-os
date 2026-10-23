#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
run_dir="$root/.local/run"
stop_process() {
  local name="$1" pid_file="$run_dir/$1.pid"
  [[ -f "$pid_file" ]] || return 0
  local pid args; pid="$(cat "$pid_file")"
  if [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null; then
    args="$(ps -p "$pid" -o args= 2>/dev/null || true)"
    case "$name:$args" in
      api:*lynx-api*|web:*next*)
        kill "$pid"
        for _ in $(seq 1 10); do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
        if kill -0 "$pid" 2>/dev/null; then kill -KILL "$pid"; fi
        echo "$name encerrado." ;;
      *) echo "PID salvo para $name não corresponde mais ao serviço; não foi encerrado." ;;
    esac
  fi
  rm -f "$pid_file"
}
stop_process web
stop_process api
docker compose -f "$root/backend/compose.yaml" down
echo "Serviços encerrados. Volumes PostgreSQL preservados."
