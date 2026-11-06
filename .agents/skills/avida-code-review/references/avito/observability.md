# Avito Observability (logs, metrics, alerts, graceful degradation)

**Activates when**: diff touches code that logs/emits metrics, or modifies `alert.yaml`, or modifies `app.toml [[dependencies]]`.

**Used by**: `qa-expert` (logs/metrics/spans/alerts/GD), `security-auditor` (PII in logs/metrics/spans).

Out of scope: Sentry config, NFR/SLA (see `nfr.md`).

## CRITICAL checks

### 1. No PII / no high-cardinality in metrics

Banned in metric names and tags:
- IDs (order_id, user_id, item_id), UUIDs
- URLs (full path, especially with query)
- Hashes, tokens, IPs
- Timestamps
- PII (email, phone, name, address)

```go
// CRITICAL: ID in metric name → cardinality explosion
metricName := fmt.Sprintf("order.handler.%s.process", orderID)

// CRITICAL: PII in tag
statsd.Increment("user.process.count", statsd.Tag("email", email))

// OK: bounded-cardinality tags only
statsd.Increment("handler.request.count",
    statsd.Tag("method", req.Method),     // GET/POST — finite
    statsd.Tag("status", req.StatusCode), // 200/4xx/5xx — finite
)
```

→ **P0** (operational + data-leak risk).

### 2. No PII in logs

Banned in log fields and formatted strings:
- HTTP request/response body
- Authorization headers
- Personal data (email, phone, FIO, address)
- Passwords, tokens
- Card numbers / banking data

```go
// CRITICAL
body, _ := io.ReadAll(r.Body)
log.WithField(ctx, "body", string(body))

log.WithField(ctx, "authorization", r.Header.Get("Authorization"))

// WARN: full URL may contain tokens in query
log.WithField(ctx, "url", r.URL.String())

// OK
log.WithFields(ctx, map[string]any{
    "method":     r.Method,
    "path":       r.URL.Path,  // path only
    "user_id":    userID,
    "request_id": requestID,
})
```

`r.Body` / `response.Body` / `Authorization` in logs → **P0**.

## WARNING checks

### 3. Platform logging library

Go: `go.avito.ru/gl/logger/v3`. PHP: Monolog. Python: structured logging.

Standard `log`, bare `fmt.Print*` for logging → P2.

### 4. Log levels

| Level | Use for | Frequency |
|---|---|---|
| DEBUG | dev tracing | frequent in dev, rare in prod |
| INFO | normal business events | frequent |
| WARNING | unexpected non-errors, business validation failures | rare |
| ERROR | critical, on-call attention | very rare |

`log.Error` on a business validation (`invalid order`) → P2 (misleading on-call signal). Use `log.Warn`.

### 5. Context propagation

Log methods take `ctx` first; trace_id / x-route extracted automatically.

```go
// GOOD
ctx = log.WithField(ctx, "order_id", orderID)
log.Info(ctx, "processing order")

// BAD: dropped trace_id
log.Info(context.Background(), "processing order", "order_id", orderID)
```

### 6. Platform metrics library

Go: `go.avito.ru/gl/statsd`. PHP: `avito/statsd-bundle`. Python: `paas-statsd-client`.

Manual `METRICS_HOST` / `METRICS_PORT` env vars or non-platform clients → P2.

### 7. Metric naming

Format: `<namespace>.<section>.<target>.<action>`

| Component | Example |
|---|---|
| namespace | `billing`, `order`, `user` |
| section | `handler`, `repo`, `rpc`, `db` |
| target | `create_order`, `payment` |
| action | `count`, `duration`, `errors` |

Generic names (`count`, `m1`, `order_process`) → P3.

### 8. Instrumentation coverage

| Operation | Required metrics |
|---|---|
| HTTP handler | count, duration, errors |
| RPC | count, duration, errors |
| DB query | count, duration, errors |
| Queue consumer | count, duration, errors, lag |
| External API | count, duration, errors |

Critical operation with no error metric → P2 (incidents become invisible).

### 9. Graceful degradation

Read `app.toml [[dependencies]]` to know external dependencies. Each external dep needs a fallback path (cache, default, alternative source) + GD signal.

```go
info, err := s.userClient.GetUserInfo(ctx, userID)
if err != nil {
    graceful.Degraded(graceful.Operationf("user_service__get_info"))
    if cached != nil {
        return cached, nil
    }
    return s.getDefaultUserInfo(userID), nil
}
```

GD operation name format: `service-name__operation` (double underscore). No fallback or no GD signal → P2.

### 10. Tracing / spans

Avito Go services use OpenTelemetry-compatible tracing via `go.avito.ru/gl/tracing` (or the platform's current package — check `app.toml [[dependencies]]`). Reviewer rules:

**No PII / no high-cardinality in span attributes** — same rules as metrics tags. Banned:
- IDs in span **names** (`order.process.42` — cardinality explosion in the trace UI).
- PII in attributes (`user.email`, `user.phone`, `auth.token`).
- Full URLs with query string (may carry tokens).

```go
// BAD: ID in span name
ctx, span := tracer.Start(ctx, fmt.Sprintf("process_order_%d", orderID))

// BAD: PII attribute
span.SetAttributes(attribute.String("user.email", email))

// GOOD: stable name + bounded-cardinality attrs
ctx, span := tracer.Start(ctx, "service.order.process")
span.SetAttributes(
    attribute.Int64("order.id", orderID),  // numeric ID as attribute, not name
    attribute.String("status", status),     // bounded
)
defer span.End()
```

**Span naming**: `<service>.<section>.<operation>` — same family as metric names. Generic names (`process`, `handle`) → P3.

**Span coverage**: every external IO (RPC, DB query, HTTP call to another service, queue publish) should be wrapped in a span. Critical operation without a span → P2 (incidents become invisible in distributed traces). The platform middlewares usually auto-instrument HTTP/RPC; manual code that bypasses them (raw `net/http.Client.Do` to an internal service) → P2.

**Error recording**: errors must be attached to the span (`span.RecordError(err)` + `span.SetStatus(codes.Error, ...)`), not just logged. A span that succeeds while the operation returned an error → P2 (false-green traces).

**Baggage / context propagation**: when a goroutine outlives the parent request, the span context must be carried via the propagated `context.Context` — not via a stored field. A goroutine that constructs a fresh `context.Background()` for tracing → P2 (orphan span, broken trace chain).

### 11. `alert.yaml` coverage

Critical metrics must have alerts:
- Error rate
- Latency p95/p99
- Availability
- Resource usage (if relevant)

`alert.yaml` missing entirely on a prod service → P1. Critical metric without alert → P2.

```yaml
triggers:
  - name: high_error_rate
    expression: "sum(rate(service.handler.*.errors[5m])) > 100"
    severity: critical
    description: "High error rate"
```

## Anti-patterns

- Per-iteration logging in loops → P2 (log spam). Use aggregated `"count": len(items)` instead.
- Error path with no metric → can't alert on the failure mode.
- Secret in metric tag: `fmt.Sprintf("auth.token.%s.count", token)` → **P0**.

## Quick checklist

1. No PII / IDs / URLs in metric names or tags?
2. No PII / body / Authorization in log fields?
3. Platform logger / statsd client used?
4. Log levels appropriate (ERROR for critical only)?
5. Context propagated to log calls?
6. Metric names follow `<ns>.<section>.<target>.<action>`?
7. Critical operations instrumented (count + duration + errors)?
8. External deps have fallback + `graceful.Degraded`?
9. Spans on external IO, no PII in span names/attrs, errors recorded on span?
10. `alert.yaml` covers error rate / latency / availability?
11. No per-iteration log spam?
