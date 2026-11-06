# Avito Security (auth.toml, X-Source, Data Bus auth)

**Activates when**: `auth.toml` exists or is modified; or X-Source header is referenced in Go/Python code.

**Used by**: `security-auditor`.

Out of scope here (handled by other tooling):
- mTLS (on by default platform-wide)
- Hardcoded secrets (universal security-auditor + scanners)
- SQL injection / XSS (avito lint, generic security-auditor)
- Dependency CVEs (SAST)

## CRITICAL checks

### 1. `auth.toml` exists

Every service with public RPC/REST endpoints needs `auth.toml`. Absent file on such a service → **P0**.

Minimal required structure:

```toml
version = "1.0"

[default]
policy = "deny"

[[policy]]
description = "Why this access is granted"
endpoints = ["rpc:getUserInfo", "rpc:createOrder"]
clients = ["billing-service", "user-api"]
```

`[default] policy = "deny"` is the least-privilege baseline. Anything else → **P0**.

### 2. No wildcard clients on sensitive endpoints

```toml
# CRITICAL: opens delete to anyone
[[policy]]
endpoints = ["rpc:deleteUser"]
clients = ["*"]

# CRITICAL: opens everything to everyone
[[policy]]
endpoints = ["*"]
clients = ["*"]
```

`clients = ["*"]` or `endpoints = ["*"]` on any non-trivial endpoint → **P0**. Public-internet endpoints use `clients = ["@public"]` deliberately — verify the endpoint is intended to be public.

### 3. X-Source must not drive business logic

X-Source identifies the calling service (validated by mTLS). It is **only** for observability — metrics, logs, tracing tags.

Findings:

```go
// CRITICAL: authz via X-Source — bypass auth.toml
if r.Header.Get("X-Source") != "admin-panel" {
    http.Error(w, "forbidden", 403)
    return
}

// CRITICAL: branch business logic by source
if source == "premium-service" {
    return h.premiumProcess()
}
return h.regularProcess()
```

Any `if X-Source ==` that affects authz or business behaviour → **P0**.

```go
// OK: X-Source as a metric tag / log field
statsd.Increment("handler.process.count", statsd.Tag("source", source))
log.Info(ctx, "request", "source", source)
```

## WARNING checks

### 4. Policy descriptions

Every `[[policy]]` block needs a `description`. Absent → P2 (operational, makes audit hard).

### 5. Endpoint naming

- RPC: `"rpc:methodName"`
- HTTP: `"http:METHOD:/path"` — method required.
- Regex: `"rpc:admin*"` — flag unless the regex is clearly intentional and the description justifies it.

Typos (method exists in `service.brief` but `auth.toml` references a different camel-cased name) → P1.

### 6. Data Bus authorization

```toml
auth_events = true

[[policy]]
events = ["order.created"]
clients = ["order-service"]  # producer

[[policy]]
events = ["order.created"]
clients = ["billing-service", "notification-service"]  # consumers
```

Findings:
- `events = ["*"]` anywhere → **P0**.
- `events` entries present but `auth_events = true` missing → P1 (policies inert).
- `auth_events = true` with no `events` policies → P2 (default-deny means nothing can produce/consume, which is valid but suspicious; verify intent).

## Quick checklist

1. `auth.toml` exists for any service with public endpoints?
2. `[default] policy = "deny"`?
3. No `clients = ["*"]` on sensitive endpoints?
4. No `endpoints = ["*"]`?
5. X-Source used only for metrics/logs/tracing — never authz, never business branching?
6. Every `[[policy]]` has `description`?
7. Endpoint names match `brief/rpc/*` actual method names?
8. Data Bus: `auth_events = true` when `events` policies exist?
