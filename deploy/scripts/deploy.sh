#!/usr/bin/env bash
# Executed on the VPS from /opt/clinic-schedule/.releases/<tag>/deploy.sh.
# Needs Docker Compose v2, flock, and a deploy user with Docker access.
set -Eeuo pipefail

APP_DIR="/opt/clinic-schedule"
RELEASE_TAG="${1:-}"
GHCR_USER="${2:-}"

if [[ ! "$RELEASE_TAG" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "Expected a stable SemVer release such as v1.0.0." >&2
  exit 2
fi
if [[ -n "$GHCR_USER" && ! "$GHCR_USER" =~ ^[a-zA-Z0-9-]+$ ]]; then
  echo "Invalid GHCR account name." >&2
  exit 2
fi

RELEASE_DIR="$APP_DIR/.releases/$RELEASE_TAG"
ENV_FILE="$APP_DIR/.env"
COMPOSE_FILE="$APP_DIR/docker-compose.prod.yml"
CADDY_FILE="$APP_DIR/deploy/Caddyfile"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE; bootstrap the server configuration before deploying." >&2
  exit 1
fi
for file in docker-compose.prod.yml Caddyfile backup.sh deploy.sh; do
  if [[ ! -f "$RELEASE_DIR/$file" ]]; then
    echo "Release is incomplete: $RELEASE_DIR/$file" >&2
    exit 1
  fi
done
if ! command -v docker >/dev/null || ! docker compose version >/dev/null 2>&1; then
  echo "Docker Engine and Compose v2 are required." >&2
  exit 1
fi
if ! command -v flock >/dev/null; then
  echo "flock (util-linux) is required." >&2
  exit 1
fi

umask 077
mkdir -p "$APP_DIR/deploy"
exec 9>"$APP_DIR/.deploy.lock"
flock -w 300 9 || { echo "Another deployment is still running." >&2; exit 1; }

backup_dir=""
docker_config_dir=""
changed=false
had_compose=false
had_caddy=false
had_backup_script=false

compose() {
  docker compose --project-directory "$APP_DIR" --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

finish() {
  local result="$1"
  trap - EXIT

  if [[ "$result" -ne 0 && "$changed" == true ]]; then
    echo "Release $RELEASE_TAG failed. Restoring the previous configuration..." >&2
    cp -p -- "$backup_dir/.env" "$ENV_FILE"

    if [[ "$had_compose" == true ]]; then
      cp -p -- "$backup_dir/docker-compose.prod.yml" "$COMPOSE_FILE"
    fi
    if [[ "$had_caddy" == true ]]; then
      cp -p -- "$backup_dir/Caddyfile" "$CADDY_FILE"
    fi
    if [[ "$had_backup_script" == true ]]; then
      cp -p -- "$backup_dir/backup.sh" "$APP_DIR/deploy/backup.sh"
    else
      rm -f -- "$APP_DIR/deploy/backup.sh"
    fi

    if [[ "$had_compose" == true ]]; then
      if ! (compose pull backend frontend && compose up -d --no-build --wait --wait-timeout 240); then
        echo "Automatic rollback could not restore healthy services. Inspect docker compose logs." >&2
      else
        echo "Previous release restored." >&2
      fi
    else
      # First installation: leave persistent volumes intact, even if startup failed.
      if [[ -f "$COMPOSE_FILE" ]]; then
        compose down || true
      fi
      rm -f -- "$COMPOSE_FILE"
    fi
    if [[ "$had_caddy" == false ]]; then
      rm -f -- "$CADDY_FILE"
    fi
  fi

  if [[ -n "$backup_dir" ]]; then
    rm -rf -- "$backup_dir"
  fi
  if [[ -n "$docker_config_dir" ]]; then
    rm -rf -- "$docker_config_dir"
  fi
  exit "$result"
}
trap 'finish $?' EXIT

for key in POSTGRES_PASSWORD APP_DOMAIN ACME_EMAIL BASIC_AUTH_USER BASIC_AUTH_HASH; do
  if ! grep -qE "^${key}=.+" "$ENV_FILE"; then
    echo "Required value $key is missing from the VPS .env." >&2
    exit 1
  fi
done

# GitHub Actions supplies a short-lived read-only GHCR credential via SSH stdin.
# Its Docker config is private and deleted on any exit. No PAT is stored on VPS.
if [[ -n "$GHCR_USER" ]]; then
  docker_config_dir="$(mktemp -d "$APP_DIR/.ghcr-auth.XXXXXXXX")"
  chmod 700 "$docker_config_dir"
  export DOCKER_CONFIG="$docker_config_dir"
  docker login ghcr.io --username "$GHCR_USER" --password-stdin >/dev/null
fi

backup_dir="$(mktemp -d "$APP_DIR/.rollback.XXXXXXXX")"
chmod 700 "$backup_dir"
cp -p -- "$ENV_FILE" "$backup_dir/.env"
if [[ -f "$COMPOSE_FILE" ]]; then
  cp -p -- "$COMPOSE_FILE" "$backup_dir/docker-compose.prod.yml"
  had_compose=true
fi
if [[ -f "$CADDY_FILE" ]]; then
  cp -p -- "$CADDY_FILE" "$backup_dir/Caddyfile"
  had_caddy=true
fi
if [[ -f "$APP_DIR/deploy/backup.sh" ]]; then
  cp -p -- "$APP_DIR/deploy/backup.sh" "$backup_dir/backup.sh"
  had_backup_script=true
fi

# pg_dump runs before the new backend starts (and applies schema migrations).
# A pre-existing but stopped database must not be upgraded without a backup.
if [[ "$had_compose" == true ]]; then
  db_id="$(compose ps --all -q db)"
  if [[ -n "$db_id" ]]; then
    if [[ "$(docker inspect -f '{{.State.Running}}' "$db_id")" != "true" ]]; then
      echo "Database exists but is not running; resolve this before deploying." >&2
      exit 1
    fi
    sh "$RELEASE_DIR/backup.sh"
  fi
fi

# Keep the existing secret values and file permissions. Do not source .env
# because passwords and bcrypt strings are not necessarily valid shell syntax.
next_env="$(mktemp "$APP_DIR/.env.next.XXXXXXXX")"
awk -v tag="$RELEASE_TAG" '
  BEGIN { seen=0 }
  /^IMAGE_TAG=/ { if (!seen) { print "IMAGE_TAG=" tag; seen=1 }; next }
  { print }
  END { if (!seen) print "IMAGE_TAG=" tag }
' "$ENV_FILE" > "$next_env"
chmod --reference="$ENV_FILE" "$next_env"

changed=true
mv -f -- "$next_env" "$ENV_FILE"
cp -p -- "$RELEASE_DIR/docker-compose.prod.yml" "$COMPOSE_FILE"
cp -p -- "$RELEASE_DIR/Caddyfile" "$CADDY_FILE"
cp -p -- "$RELEASE_DIR/backup.sh" "$APP_DIR/deploy/backup.sh"

compose config --quiet
compose pull backend frontend
compose up -d --no-build --wait --wait-timeout 240
echo "Release $RELEASE_TAG is running and Compose health checks passed."
