# GitHub tag release workflow

The production release workflow is in `.github/workflows/deploy.yml`.

1. Push a **stable SemVer tag** such as `v1.0.0` pointing to a commit on `main`. Only a tag push triggers a production release; branch pushes do not.
2. GitHub Actions validates the tag and its ancestry, runs `go test ./...` and `go build ./cmd/api`, then installs frontend dependencies from the frozen lockfile and runs TypeScript, ESLint, and Next.js build.
3. If checks pass, images are built for `linux/amd64` and published to `ghcr.io/kuminovvv/clinic-schedule-backend:v1.0.0` and `ghcr.io/kuminovvv/clinic-schedule-frontend:v1.0.0`. Jobs use `GITHUB_TOKEN` with `packages: write`. Already published tags are reused only when their `org.opencontainers.image.revision` OCI label matches the release commit; the workflow never knowingly overwrites existing release images.
4. When configured, the deployment job SSHes to the VPS with its pinned host key, stages the Compose file, Caddyfile, backup script and deploy script under `/opt/clinic-schedule/.releases/v1.0.0/`, then invokes `deploy.sh v1.0.0`. The job passes its short-lived `GITHUB_TOKEN` through SSH stdin to the remote `docker login --password-stdin` and removes the temporary Docker login configuration after deployment. This permits GHCR packages to remain private.
5. The server pulls both GHCR images before downtime, stops frontend and backend, and creates a SQL backup with both writers stopped before the new backend runs migrations. It keeps its existing private `/opt/clinic-schedule/.env`, updates only `IMAGE_TAG`, and starts services using `docker compose up --wait`. On an unsuccessful upgrade, the script stops both application services before restoring the previous configuration and restarting the previous release. No step removes Docker volumes.

## Configure GitHub

Set the following **GitHub Actions environment secrets** for environment `production` (repository-level secrets are also supported):

| Secret | Value |
| --- | --- |
| `DEPLOY_HOST` | VPS public DNS name or address |
| `DEPLOY_USER` | Unix user that owns `/opt/clinic-schedule` and can use Docker Compose |
| `DEPLOY_SSH_KEY` | Private SSH key paired with the VPS deploy user's authorized public key |
| `DEPLOY_KNOWN_HOSTS` | Verified server SSH host key line (obtain its fingerprint via a trusted channel before adding; hashed known_hosts lines are supported) |

The workflow does not read or transmit the VPS `.env` or its database password. Configure that file directly on the server. See `docs/DEPLOYMENT.md` for initial VPS setup, HTTPS, credentials, and DNS.

Until all four SSH secrets are set, the workflow can run checks and publish images, but it **skips the VPS connection**. The deployment job may also be gated by environment protection rules. The deploy job uses its short-lived `GITHUB_TOKEN` with `packages: read` to pull private GHCR packages. Manual rollbacks require the images to remain cached locally, or temporary GHCR read credentials if they must be pulled again.

## Manual rollback

Releases remain staged on the VPS for later rollback. To restore a previous published tag:

```bash
bash /opt/clinic-schedule/.releases/v1.0.0/deploy.sh v1.0.0
```

Do not re-point an existing Git tag; make a new release tag for code changes. A deployment rollback only changes containers and configuration. Database migrations that already ran are **not** reversed; migrations must be backward-compatible. Keep external database backups before schema-changing releases.

GHCR tags are treated as write-once by this pipeline. They are not inherently content-addressed or protected against changes made outside this workflow. For stronger guarantees, configure GitHub tag rulesets and GHCR package access restrictions.
