#!/bin/sh
set -eu

# Local server-only backup. The volume is never deleted by the deploy workflow.
# Run from /opt/clinic-schedule as a daily cron job; copy backups off-server too.
cd /opt/clinic-schedule
umask 077
mkdir -p backups
stamp=$(date -u +%Y%m%dT%H%M%SZ)
output="backups/clinic-${stamp}.sql.gz"
temp="${output}.partial"
temp_sql="backups/.clinic-${stamp}.sql.partial"
trap 'rm -f "$temp" "$temp_sql"' EXIT HUP INT TERM

# Don't pipe pg_dump directly to gzip in POSIX sh: a failed pg_dump could
# otherwise leave a nonempty but useless compressed file.
docker compose --env-file .env -f docker-compose.prod.yml exec -T db \
  pg_dump -U clinic_schedule -d clinic_schedule --no-owner --no-acl > "$temp_sql"
test -s "$temp_sql"
gzip -9 -c "$temp_sql" > "$temp"
gzip -t "$temp"
rm -f "$temp_sql"
mv "$temp" "$output"
trap - EXIT HUP INT TERM
find backups -maxdepth 1 -type f -name 'clinic-*.sql.gz' -mtime +13 -delete
echo "Backup saved: $output"
