#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
run_dir="$root/.local/run"; log_dir="$root/.local/logs"
mkdir -p "$run_dir" "$log_dir" "$root/.local/bin"
chmod 700 "$root/.local" "$run_dir"
docker volume create lynx-os-dev-pgdata >/dev/null
docker compose -f "$root/backend/compose.yaml" up -d --wait
"$root/scripts/with-backend-env.sh" "$root/backend/scripts/migrate-postgres.sh"
(cd "$root/backend" && go build -o "$root/.local/bin/lynx-api" ./cmd/http)
start_process() {
  local name="$1"; shift; local pid_file="$run_dir/$name.pid"
  if [[ -f "$pid_file" ]]; then
    local old_pid old_args; old_pid="$(cat "$pid_file")"
    if [[ "$old_pid" =~ ^[0-9]+$ ]] && kill -0 "$old_pid" 2>/dev/null; then
      old_args="$(ps -p "$old_pid" -o args= 2>/dev/null || true)"
      case "$name:$old_args" in
        api:*lynx-api*)
          running_inode="$(stat -Lc '%i' "/proc/$old_pid/exe" 2>/dev/null || true)"
          current_inode="$(stat -Lc '%i' "$root/.local/bin/lynx-api" 2>/dev/null || true)"
          if [[ -n "$running_inode" && "$running_inode" == "$current_inode" ]]; then
            echo "$name já está ativo e atualizado (PID $old_pid)."; return
          fi
          echo "API compilada novamente; reiniciando o processo antigo (PID $old_pid)."
          kill "$old_pid"
          for _ in $(seq 1 10); do kill -0 "$old_pid" 2>/dev/null || break; sleep 1; done
          if kill -0 "$old_pid" 2>/dev/null; then kill -KILL "$old_pid"; fi
          rm -f "$pid_file"
          ;;
        web:*next*) echo "$name já está ativo (PID $old_pid)."; return ;;
      esac
    fi
    rm -f "$pid_file"
  fi
  if [[ "$name" == api ]] && curl --silent --fail http://127.0.0.1:8082/health/ready >/dev/null 2>&1; then
    echo "API já está respondendo em http://localhost:8082; processo existente mantido."
    return
  fi
  if [[ "$name" == web ]] && curl --silent --fail http://127.0.0.1:3100/ >/dev/null 2>&1; then
    echo "Frontend já está respondendo em http://localhost:3100; processo existente mantido."
    return
  fi
  nohup setsid "$@" >>"$log_dir/$name.log" 2>&1 </dev/null &
  echo "$!" > "$pid_file"; echo "$name iniciado (PID $!)."
}
start_process api "$root/scripts/with-backend-env.sh" "$root/.local/bin/lynx-api"
start_process web "$root/web/node_modules/.bin/next" dev --webpack --hostname 127.0.0.1 --port 3100
for attempt in $(seq 1 30); do
  api_ok=false; web_ok=false
  curl --silent --fail http://127.0.0.1:8082/health/ready >/dev/null 2>&1 && api_ok=true
  curl --silent --fail http://127.0.0.1:3100/ >/dev/null 2>&1 && web_ok=true
  if $api_ok && $web_ok; then echo "Lynx OS: http://localhost:3100 | API: http://localhost:8082 | Mailpit: http://localhost:8025"; exit 0; fi
  sleep 1
done
echo "Algum serviço não respondeu. Consulte make logs e make status." >&2
exit 1
