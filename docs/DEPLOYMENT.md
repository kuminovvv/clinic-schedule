# Clinic Schedule: production deploy by GitHub tag

The repository is `Kuminovvv/clinic-schedule`. Release tags in the format
`v1.0.0` trigger GitHub Actions to test the app, build both images in GitHub,
publish them to GHCR and update the VPS over SSH. **No Next.js/Go build runs
on the 4 GB VPS.** Routine deployment requires no SSH login by the developer.

## Infrastructure

- Ubuntu VPS with Docker Engine + Docker Compose v2, 4 vCPU / 4 GiB RAM / 50 GiB SSD.
- Public DNS record for a domain (e.g. `clinic.example.com`) pointing to the VPS IPv4 address.
- Inbound TCP **80, 443** (plus TCP 22 from trusted administration IPs); optional UDP 443 for HTTP/3.
- `/opt/clinic-schedule/.env` (secrets, mode `0600`, kept outside Git); Docker volumes for PostgreSQL and Caddy certificates.
- `docker-compose.prod.yml` runs four containers: PostgreSQL 17, Go API, standalone Next.js and Caddy (HTTPS + Basic Auth).
- Backend and PostgreSQL have **no public ports**. The only externally reachable service is Caddy.

> **Security:** Basic Auth protects the entire site, including the API, but
> every person sharing the credentials has identical rights, including deleting
> the complete schedule. Do not store identifying patient information until
> proper individual accounts, role permissions, logging and your applicable
> data-protection requirements are implemented.

## First-time server setup (one time)

1. Install Docker Engine and Compose v2 on Ubuntu using the
   [official Docker Ubuntu instructions](https://docs.docker.com/engine/install/ubuntu/).
   Run `docker compose version` and `docker info` to verify. Add the non-root
   deploy account to the `docker` group (membership grants root-equivalent
   access). Secure SSH: key authentication only; restrict port 22 to trusted IPs
   where feasible. Ensure DNS points at the VPS before the first release.
2. As the deploy account create the deploy directory:

   ```sh
   sudo mkdir -p /opt/clinic-schedule
   sudo chown "$USER":"$USER" /opt/clinic-schedule
   chmod 700 /opt/clinic-schedule
   cd /opt/clinic-schedule
   ```

3. Create `/opt/clinic-schedule/.env` with `chmod 600` using the variables
   shown in [`.env.production.example`](../.env.production.example). **Do not
   copy local frontend/backend `.env.local` files.** `POSTGRES_PASSWORD` must
   be a long URL-safe value; `openssl rand -hex 32` is suitable.
   `APP_DOMAIN` is your real DNS hostname; `ACME_EMAIL` is the TLS contact.
   Choose a strong random password for `BASIC_AUTH_USER` and create the bcrypt
   `BASIC_AUTH_HASH` with Caddy (the cleartext password must never go into Git):

   ```sh
   read -rs -p 'Website login password: ' CLINIC_PASSWORD; echo
   # Avoid recording plaintext in shell history or a CLI argument.
   printf '%s' "$CLINIC_PASSWORD" | docker run --rm -i \
     caddy:2.10-alpine caddy hash-password
   unset CLINIC_PASSWORD
   ```

   Set the returned hash as a **single-quoted** `.env` value, e.g.
   `BASIC_AUTH_HASH='$2a$14$...'`, preserving `$` characters.
   The release workflow will set `IMAGE_TAG` when it deploys; initialize it
   with a placeholder like `v1.0.0`.
4. Generate a **separate deploy-only** Ed25519 SSH key on your own computer,
   install its public key in `~/.ssh/authorized_keys` for the VPS deploy user
   and verify login. Add these GitHub Actions repository secrets at
   [GitHub Actions secrets](https://github.com/Kuminovvv/clinic-schedule/settings/secrets/actions):

   | Secret | Value |
   | --- | --- |
   | `DEPLOY_HOST` | VPS public IP / DNS (without scheme) |
   | `DEPLOY_USER` | Non-root VPS deploy account |
   | `DEPLOY_SSH_KEY` | **Private** Ed25519 deploy key (multiline OpenSSH format) |
   | `DEPLOY_KNOWN_HOSTS` | Verified VPS SSH host-key line (check its fingerprint out of band; do **not** blindly trust `ssh-keyscan`) |

   GitHub Actions must have permissions to read repository code and publish
   GHCR packages. Never commit keys or the production `.env`.

5. Make sure your DNS A (and any AAAA) record resolves to the VPS. If using
   Cloudflare DNS for a server accessible in Russia, keep the record **DNS-only**
   rather than proxied if Cloudflare's reverse proxy is unreachable from your
   users. Caddy then issues TLS certificates directly and renews them.

## Deploy a release

For a **new demo VPS only**, the repository includes
`deploy/scripts/bootstrap-demo.sh`, which creates the server `.env`, a strong
database password and a Caddy bcrypt hash without transmitting credentials
through the terminal output or storing them in Git. After Docker and the
`clinic-deploy` account are configured, copy the script to the VPS and run as
`clinic-deploy`:

```sh
bash bootstrap-demo.sh clinic.82-114-226-100.sslip.io you@example.com demo
```

The generated one-time demo login is stored only in
`/opt/clinic-schedule/demo-access.txt` (mode 0600). Obtain it through a secure
channel and do not forward database credentials to demo users. **Do not** run
the script on an existing production installation: it refuses to overwrite
the existing `.env` or demo credentials. The free sslip.io hostname is for
testing only; use your own domain for a long-lived deployment.

### Additional customer demo logins

On the VPS, run `deploy/scripts/create-demo-accounts.sh` **once** as the
`clinic-deploy` user to create five separate Basic Auth logins named
`client01`–`client05`. It refuses to overwrite existing credentials. The
plain-text credentials are stored only in `/opt/clinic-schedule/demo-clients.txt`
(0600), while bcrypt hashes are stored in
`/opt/clinic-schedule/deploy/basic-auth-users.caddy` (0600). This private file
is mounted into Caddy and **persists across tag-based deployments**; do not add
it to Git. The original `demo` login remains usable unless intentionally
revoked. All six logins have the same permissions; these are edge HTTP Basic
Auth logins, not individual accounts with role-based permissions in the app.

Run locally **after the code is committed and pushed**:

```sh
git checkout main
git pull --ff-only
git tag -a v1.0.0 -m 'Clinic Schedule v1.0.0'
git push origin v1.0.0
```

See GitHub Actions -> `Deploy (release tag)` for the build, push and deployment
steps. Every later new `v*` semver tag triggers the same workflow. **Do not
reuse or rewrite an existing release tag.** The workflows are stored in Git;
they must be pushed to `main` before they can run.

The pipeline performs health checks before declaring success and attempts to
restore the preceding release if a deployment fails. During migrations,
**do not assume rollback can reverse PostgreSQL schema changes**; always
make a tested backup before deployments that change schema.

## Database backups and recovery

With the app up, run `sh /opt/clinic-schedule/deploy/backup.sh` to create a
gzip-compressed SQL backup. Configure a daily cron job, for example:

```cron
15 3 * * * /bin/sh /opt/clinic-schedule/deploy/backup.sh >> /opt/clinic-schedule/backup.log 2>&1
```

The script keeps 14 days of local backups. **Also copy encrypted backups to
an independent storage location**: losing a VPS can lose both its volumes and
local backups. Test restoring to a separate database before treating backups
as reliable. Do not use `docker compose down -v` on production.

## Operations / verification

```sh
cd /opt/clinic-schedule
docker compose --env-file .env -f docker-compose.prod.yml ps
docker compose --env-file .env -f docker-compose.prod.yml logs --tail 80 backend
docker compose --env-file .env -f docker-compose.prod.yml logs --tail 80 frontend
curl -I https://YOUR_DOMAIN/        # unauthenticated: HTTP 401 expected
```

If the VPS is provisioned but `DEPLOY_HOST`, SSH credentials or the DNS domain
are unknown, **the initial live deployment cannot be completed**; configuring
the pipeline in GitHub alone is not evidence of a live server.
