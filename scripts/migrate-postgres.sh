#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL is required (load the intended environment first)." >&2
  exit 1
fi
command -v psql >/dev/null || { echo "psql is required." >&2; exit 1; }

psql "$DATABASE_URL" -X -v ON_ERROR_STOP=1 -q -c \
  'CREATE TABLE IF NOT EXISTS lynx_schema_migrations (name TEXT PRIMARY KEY, sha256 TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())'

shopt -s nullglob
migrations=("$repo_root"/migrations/*.up.sql)
for migration in "${migrations[@]}"; do
  name="$(basename "$migration")"
  checksum="$(sha256sum "$migration" | cut -d ' ' -f 1)"
  applied="$(psql "$DATABASE_URL" -X -v ON_ERROR_STOP=1 -q -tA \
    -c "SELECT sha256 FROM lynx_schema_migrations WHERE name = '$name'")"
  if [[ -n "$applied" ]]; then
    if [[ "$applied" != "$checksum" ]]; then
      echo "Applied migration changed: $name" >&2
      exit 1
    fi
    continue
  fi

  temporary_sql="$(mktemp)"
  trap 'rm -f "$temporary_sql"' EXIT
  cat "$migration" > "$temporary_sql"
  printf "\nINSERT INTO lynx_schema_migrations(name, sha256) VALUES ('%s', '%s');\n" "$name" "$checksum" >> "$temporary_sql"
  psql "$DATABASE_URL" -X -v ON_ERROR_STOP=1 -1 -q -f "$temporary_sql"
  rm -f "$temporary_sql"
  trap - EXIT
  echo "Applied $name"
done
