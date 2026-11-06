# Telegram Chat Proxy

Single-tenant Telegram proxy for anonymous communication between external users and an internal team. Each external private chat maps to one forum topic in the configured internal supergroup.

The service uses one Telegram bot for every interaction. The bot receives private user messages, polls the internal
supergroup, creates forum topics, and copies messages in both directions.

PostgreSQL provides durable polling offsets, update deduplication, per-chat ordered operations, fenced worker leases, retries, and retained terminal failures. Delivery is at least once; a crash after Telegram accepts a request but before the database commit can produce a duplicate.

## Configuration

To keep configuration mistakes from exhausting process memory, `Inbox.BatchSize`
is capped at 1000 and `Outbox.WorkerPoolSize` is capped at 100.

Update the tracked, secret-free `config/config.yaml` for the runtime environment.
Keep credentials out of this file.

Set the PostgreSQL DSN and Telegram deployment values through environment
variables; do not put them in the configuration file:

```shell
export POSTGRES_DSN=postgres://postgres@localhost:5432/tg_bot_proxy?sslmode=disable
export TELEGRAM_BOT_TOKEN=...
export TELEGRAM_BOT_ID=...
export TELEGRAM_SUPERGROUP_ID=...
```

The bot must be a supergroup administrator with permission to manage topics.
Telegram delivers all human group messages to bot administrators, so Privacy
Mode may remain enabled.

The default local PostgreSQL database is `tg_bot_proxy` in Docker Compose, the Makefile, and the example runtime DSN.

## Local setup

```shell
make infra
make migrations-up
make xo
go run ./cmd
```

`make migrations` resets the configured local database and is destructive. It is not required for ordinary startup.

## Verification

```shell
go test ./...
make linter
```

Repository integration tests run when `TELEGRAM_PROXY_TEST_DATABASE_URL` points to an isolated migrated PostgreSQL database. Live Telegram acceptance checks require dedicated bot credentials and a test supergroup; see `docs/solution.md` section 17.

Terminal inbox and outbox failures remain in PostgreSQL for manual investigation. A failed outbox head blocks later operations for that chat only; the MVP intentionally has no administrative repair UI.

## Deployment

The Docker Compose and GitHub Actions deployment is documented in
[`deploy/README.md`](deploy/README.md). GitHub Actions publishes an image to GHCR
and deploys it to the VPS. Runtime secrets stay in a protected environment file
on the VPS.
