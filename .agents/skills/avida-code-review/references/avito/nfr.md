# NFR — Non-Functional Requirements (Avito PaaS)

**Activates when**: `nfr.toml` exists or is modified.

**Used by**: `architect-reviewer`, `qa-expert`.

`nfr-validate` already checks schema validity, presence of mandatory fields, duplicate aliases, etc. This file covers **logical applicability** — values that pass the linter but indicate problems.

## CRITICAL checks

### 1. `nfr.toml` exists for prod services

Linter skips when file is absent (`exit 0`). A prod service with no `nfr.toml` → P1 (no SLI baseline).

## WARNING checks

### 2. SLO realism

```toml
# BAD: unreachable
[service]
slo = 100
```

100% is unreachable. Typical values:

| SLO | Downtime budget | Service tier |
|---|---|---|
| 99.99% | ~4 min/month | critical (payments, auth) |
| 99.9% | ~43 min/month | high (core services) |
| 99% | ~7 h/month | medium (internal tools) |
| 95% | ~36 h/month | low (non-critical) |

SLO = 100 → P2 (impossible target distorts SLIs). SLO ≤ 95% on a critical/core service → P2 (requires justification).

### 3. Default latency realism

```toml
[default.all]
latency = "30s"  # suspicious
```

Typical default: 50ms — 2s. If default latency > 5s, expect a corresponding override on heavy operations; missing override → P2.

### 4. `error_codes`

```toml
[default.all]
error_codes = []  # nothing counts as error → masks real problems
```

Empty `error_codes` → P2 unless explicitly justified (e.g., internal-only service where all responses are success).

### 5. `skip = true` on business endpoints

OK: health checks, `/_info`, `/metrics`.

```toml
# BAD: business endpoint skipped from SLI calculations
[[rest]]
path = "/api/users_GET"
skip = true
```

`skip = true` on a non-technical endpoint → P1 (silent SLI gap).

### 6. `ignoreStatuses` with 5xx

```toml
# CRITICAL: 5xx silently dropped from SLI
[service]
ignoreStatuses = ["500", "502", "503"]
```

Any 5xx in `ignoreStatuses` → **P1**. Legitimate use only for 4xx (404 not-found, 422 validation, 429 rate-limited).

### 7. Deprecated modes

```toml
[service]
modes = ["ignoreUndefinedNotFound"]  # DEPRECATED
```

`ignoreUndefinedNotFound` → P2 (deprecated). Migrate to `ignoreStatuses = ["404"]`.

`ignoreUndefinedLimit` for legacy is acceptable.

### 8. Heavy operations vs default latency

If default latency is large (> 5s) but no `[[rpc]]` / `[[rest]]` overrides describe lighter operations, that's suspicious — either the default is wrong, or heavy operations should be hoisted into explicit sections.

```toml
# GOOD: small default + explicit heavy ops
[default.all]
latency = "100ms"
rpm = 1000
error_codes = ["5**"]

[[rpc]]
method = "generateReport"
latency = "30s"
rpm = 10
```

### 9. Operations contribute to SLI

Every `[[operation]]` impacts overall SLI. Sanity-check:

| Operation kind | Typical latency | Typical rpm |
|---|---|---|
| Background sync | 1s — 5m | 1 — 100 |
| Batch processing | 1m — 30m | 1 — 10 |
| Cache refresh | 100ms — 1s | 1 — 1000 |
| DB migration | 1m — 1h | < 1 |

Suspicious values:
- Latency > 1h → operation should be split.
- RPM > 100k for a background op → likely wrong.
- Latency = 1ms for an obviously heavy op → unrealistic.

## Quick checklist

1. `nfr.toml` exists?
2. SLO < 100 and matches service criticality?
3. Default latency realistic (50ms–2s typical)?
4. `error_codes` not empty (or justified)?
5. `skip = true` only on technical endpoints?
6. No 5xx in `ignoreStatuses`?
7. No deprecated `ignoreUndefinedNotFound`?
8. Heavy ops broken out into explicit `[[rpc]]` / `[[rest]]` / `[[operation]]`?
9. Operation latency/rpm realistic for the operation kind?
