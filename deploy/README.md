# Docker deployment to a VPS

GitHub Actions tests the service, publishes a commit-addressed image to GitHub
Container Registry (GHCR), and deploys that image with Docker Compose. Runtime
secrets remain on the VPS and are never copied into an image or GitHub Actions.

The deployment expects the existing `shared-services` Docker network and the
PostgreSQL container reachable as `shared-postgres:5432`.

## 1. Prepare the VPS once

Create a dedicated SSH deployment user if one does not already exist:

```shell
sudo useradd --create-home --shell /bin/bash deploy
sudo usermod --append --groups docker deploy
sudo install -d -o deploy -g deploy -m 750 /opt/tg-bot-proxy
```

Membership in the Docker group is effectively root access. Use a dedicated SSH
key for this account and do not reuse a personal key. Log out and reconnect after
adding the group, then verify:

```shell
id
docker version
docker compose version
```

Create the application secret file on the VPS:

```shell
install -o deploy -g deploy -m 600 /dev/null /opt/tg-bot-proxy/secrets.env
nano /opt/tg-bot-proxy/secrets.env
```

```dotenv
POSTGRES_DSN=postgres://postgres:URL_ENCODED_PASSWORD@shared-postgres:5432/tg_bot_proxy?sslmode=disable
TELEGRAM_BOT_TOKEN=...
TELEGRAM_BOT_ID=...
TELEGRAM_SUPERGROUP_ID=...
```

Percent-encode reserved characters in the PostgreSQL password. Do not commit
this file or copy its contents into GitHub secrets.

If the repository or GHCR package is private, authenticate the `deploy` user to
GHCR once using a token with read-only package access:

```shell
docker login ghcr.io
```

## 2. Configure GitHub

Create a GitHub environment named `production`, restrict it to the `main`
branch, and add:

- `VPS_HOST`: VPS hostname or IP address.
- `VPS_PORT`: SSH port.
- `VPS_USER`: SSH deployment user, for example `deploy`.
- `VPS_SSH_KEY`: the dedicated private deployment key.
- `VPS_KNOWN_HOSTS`: a pinned, independently verified SSH host-key line.

The workflow publishes both the immutable commit SHA tag and `latest`, but the
VPS always deploys the SHA tag. If health checks fail, it restores the previous
image tag.

## 3. Apply database migrations

Migrations are deliberately separate from application deployment. Establish an
SSH tunnel from the development machine:

```shell
ssh -L 15432:127.0.0.1:5432 deploy@VPS_HOST
```

In another local terminal, from the repository root:

```shell
read -rsp "Database password: " PGPASSWORD
echo
export PGPASSWORD
DB_PORT=15432 DB_USER=postgres DB_NAME=tg_bot_proxy make migrations-up
unset PGPASSWORD
```

Only run `migrations-up`. The `migrations` and `migrations-reset` targets are
destructive.

## 4. Deploy and operate

Push to `main` or manually start the `Deploy production` GitHub Actions
workflow. On the VPS:

```shell
cd /opt/tg-bot-proxy
docker compose --env-file .deploy.env ps
docker compose --env-file .deploy.env logs --follow app
curl --fail http://127.0.0.1:9090/healthcheck/ready
```

The application and `shared-postgres` share only the external
`shared-services` network. No application traffic port is exposed publicly;
only the monitoring endpoint is bound to VPS loopback.
