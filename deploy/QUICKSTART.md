# Deploy to a VPS with GitHub Actions

## 1. Prepare the VPS

Install Docker and Docker Compose. Add swap if the VPS has limited RAM.

Create the shared Docker network and connect PostgreSQL:

```shell
docker network create shared-services
docker network connect shared-services shared-postgres
```

Prepare the application directory:

```shell
sudo install -d -o deploy -g deploy -m 750 /opt/tg-bot-proxy
sudo install -o deploy -g deploy -m 600 /dev/null \
  /opt/tg-bot-proxy/secrets.env
```

Add runtime secrets to `/opt/tg-bot-proxy/secrets.env`:

```dotenv
POSTGRES_DSN=postgres://postgres:URL_ENCODED_PASSWORD@shared-postgres:5432/tg_bot_proxy?sslmode=disable
TELEGRAM_BOT_TOKEN=...
TELEGRAM_BOT_ID=...
TELEGRAM_SUPERGROUP_ID=...
```

## 2. Configure deployment SSH access

Generate a dedicated key locally:

```shell
ssh-keygen -t ed25519 \
  -f ~/.ssh/tg-bot-proxy-deploy \
  -C "github-actions tg-bot-proxy" \
  -N ""
```

Install it using the VPS SSH port:

```shell
ssh-copy-id -p SSH_PORT \
  -i ~/.ssh/tg-bot-proxy-deploy.pub \
  deploy@VPS_HOST
```

## 3. Configure GitHub

Create a GitHub environment named `production` and add these environment
secrets:

```text
VPS_HOST
VPS_PORT
VPS_USER
VPS_SSH_KEY
VPS_KNOWN_HOSTS
```

Generate the known-hosts value:

```shell
ssh-keyscan -p SSH_PORT -H -t ed25519 VPS_HOST
```

Use `deploy` as `VPS_USER`. Store the private deployment key as `VPS_SSH_KEY`.

## 4. Authenticate the VPS to GHCR

Create a classic GitHub personal access token with only `read:packages`. On the
VPS, log in as `deploy` without using `sudo`:

```shell
read -rsp "GHCR token: " GHCR_TOKEN
echo
printf '%s' "$GHCR_TOKEN" | \
  docker login ghcr.io -u GITHUB_USERNAME --password-stdin
unset GHCR_TOKEN
```

## 5. Apply migrations

Apply pending migrations before deploying code that requires them. Never run
the destructive migration-reset target against production.

## 6. Deploy

Push to `main`. GitHub Actions will:

1. Run tests.
2. Build the Docker image.
3. Push the commit-addressed image to GHCR.
4. Upload the Compose and application configuration files.
5. Pull and start the image on the VPS.
6. Check application health and restore the previous image on failure.

## 7. Verify

```shell
cd /opt/tg-bot-proxy
docker compose --env-file .deploy.env ps
docker compose --env-file .deploy.env logs --tail=100 app
curl --fail http://127.0.0.1:9090/healthcheck/ready
```
