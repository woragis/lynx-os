#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
backup_only=false
if [[ "${1:-}" == "--backup-only" ]]; then backup_only=true; fi
if $backup_only; then
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  backup_dir="$root/.local/backups/env/$timestamp"
  mkdir -p "$root/.local/backups/env"
  chmod 700 "$root/.local" "$root/.local/backups" "$root/.local/backups/env"
  found=false
  for relative in backend/.env web/.env; do
    target="$root/$relative"
    if [[ -f "$target" ]]; then
      mkdir -p "$backup_dir/$(dirname "$relative")"
      cp -p "$target" "$backup_dir/$relative"
      chmod 600 "$backup_dir/$relative"
      echo "Backup criado para $relative em .local/backups/env/$timestamp/"
      found=true
    fi
  done
  if ! $found; then echo "Nenhum .env encontrado para backup."; fi
  echo "Backup concluído; nenhum .env foi alterado."
  exit 0
fi

if [[ ! -f "$root/backend/.env" ]]; then
  cp "$root/backend/.env.example" "$root/backend/.env"
  chmod 600 "$root/backend/.env"
  jwt="$(openssl rand -base64 48 | tr -d '\n')"
  encryption="$(openssl rand -hex 32)"
  sed -i -e "s|^LYNX_JWT_SIGNING_KEY=.*|LYNX_JWT_SIGNING_KEY=$jwt|" -e "s|^LYNX_ENCRYPTION_KEY=.*|LYNX_ENCRYPTION_KEY=$encryption|" -e 's|^LYNX_PUBLIC_URL=.*|LYNX_PUBLIC_URL=http://localhost:3100|' "$root/backend/.env"
  echo "Criado backend/.env com segredos locais novos."
else echo "backend/.env existente preservado."; fi
if [[ ! -f "$root/web/.env" ]]; then
  cp "$root/web/.env.example" "$root/web/.env"
  chmod 600 "$root/web/.env"
  echo "Criado web/.env a partir do exemplo."
else echo "web/.env existente preservado."; fi
echo "Arquivos .env existentes são pontos fixos: setup não os substitui nem cria backups automáticos."
echo "Para guardar uma cópia manual, execute make env-backup."
