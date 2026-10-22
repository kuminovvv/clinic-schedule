#!/usr/bin/env bash
# One-time, server-side initialization for an isolated demo installation.
# No generated credentials are printed or committed to Git.
set -Eeuo pipefail

app_dir=/opt/clinic-schedule
domain="${1:-}"
acme_email="${2:-}"
login="${3:-demo}"

if [[ ! "$domain" =~ ^[a-zA-Z0-9.-]+$ || "$domain" != *.* ]]; then
  echo 'Expected a public DNS hostname as the first argument.' >&2
  exit 2
fi
if [[ ! "$acme_email" =~ ^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$ ]]; then
  echo 'Expected an ACME contact email as the second argument.' >&2
  exit 2
fi
if [[ ! "$login" =~ ^[a-zA-Z0-9_-]+$ ]]; then
  echo 'Expected a simple Basic Auth login name.' >&2
  exit 2
fi
if [[ ! -d "$app_dir" || ! -w "$app_dir" ]]; then
  echo "Missing or unwritable application directory: $app_dir" >&2
  exit 1
fi
if [[ -e "$app_dir/.env" ]]; then
  echo 'Production .env already exists; refusing to overwrite it.' >&2
  exit 1
fi
if [[ -e "$app_dir/demo-access.txt" ]]; then
  echo 'Demo credentials already exist; refusing to overwrite them.' >&2
  exit 1
fi
if ! command -v docker >/dev/null || ! command -v openssl >/dev/null; then
  echo 'Docker and openssl are required.' >&2
  exit 1
fi

umask 077
site_password="$(openssl rand -hex 20)"
db_password="$(openssl rand -hex 32)"
password_hash="$(printf '%s\n' "$site_password" | docker run --rm -i caddy:2.10-alpine caddy hash-password)"
if [[ ! "$password_hash" =~ ^\$2[aby]\$ ]]; then
  echo 'Caddy failed to generate a bcrypt hash.' >&2
  exit 1
fi

env_tmp="$(mktemp "$app_dir/.env.next.XXXXXXXX")"
credentials_tmp="$(mktemp "$app_dir/.credentials.next.XXXXXXXX")"
trap 'rm -f -- "$env_tmp" "$credentials_tmp"' EXIT
{
  printf 'APP_DOMAIN=%s\n' "$domain"
  printf 'ACME_EMAIL=%s\n' "$acme_email"
  printf 'BASIC_AUTH_USER=%s\n' "$login"
  printf "BASIC_AUTH_HASH='%s'\n" "$password_hash"
  printf 'POSTGRES_PASSWORD=%s\n' "$db_password"
  printf 'IMAGE_TAG=v1.0.0\n'
} > "$env_tmp"

printf 'URL: https://%s\nLogin: %s\nPassword: %s\n' \
  "$domain" "$login" "$site_password" > "$credentials_tmp"
chmod 600 "$env_tmp" "$credentials_tmp"

# Keep the one-time site password on the server for handover to its owner.
mv -n -- "$credentials_tmp" "$app_dir/demo-access.txt"
mv -n -- "$env_tmp" "$app_dir/.env"
echo "Demo initialized. Credentials: $app_dir/demo-access.txt (mode 600)."
