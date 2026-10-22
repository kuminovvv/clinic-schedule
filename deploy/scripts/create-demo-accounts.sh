#!/usr/bin/env bash
set -Eeuo pipefail

root=/opt/clinic-schedule
users="$root/deploy/basic-auth-users.caddy"
access="$root/demo-clients.txt"

if [[ -e "$users" || -e "$access" ]]; then
  echo 'Accounts already exist; refusing to overwrite them.' >&2
  exit 1
fi
umask 077
u_tmp="$(mktemp "$root/deploy/.users.XXXXXXXX")"
a_tmp="$(mktemp "$root/.access.XXXXXXXX")"
trap 'rm -f "$u_tmp" "$a_tmp"' EXIT

printf 'URL: https://clinic.82-114-226-100.sslip.io\n' > "$a_tmp"
for i in 1 2 3 4 5; do
  name="$(printf 'client%02d' "$i")"
  pass="$(openssl rand -hex 16)"
  hash="$(printf '%s\n' "$pass" | docker run --rm -i caddy:2.10-alpine caddy hash-password)"
  [[ "$hash" == \$2* ]] || exit 1
  printf '%s %s\n' "$name" "$hash" >> "$u_tmp"
  printf '%s: %s\n' "$name" "$pass" >> "$a_tmp"
done

chmod 600 "$u_tmp" "$a_tmp"
mv -n -- "$u_tmp" "$users"
mv -n -- "$a_tmp" "$access"
echo "Five client logins created; access details saved to $access"
