# Data Bus (Avito PaaS)

**Activates when**: `[[data-bus]]` in `app.toml`, `brief/data-bus/*.brief`, or Go/Python consumer/producer code using `databus.*` SDK.

**Used by**: `fullstack-developer`.

Data Bus is fan-out cross-service messaging. Each consumer gets its own copy. Distinct from Queues (intra-service).

## CRITICAL checks

### 1. Message size — 512 bytes max (JSON-serialized)

Messages must contain only identifiers. Consumer fetches data by ID from the source service.

```brief
// BAD: full payload, may exceed 512 bytes
message OrderCreated {
    orderId    string
    items      []OrderItem
    totalPrice int
    address    Address
}

// GOOD: identifiers only
message OrderCreated {
    orderId   string
    userId    string
    timestamp datetime
}
```

### 2. No sensitive data in payload

Data Bus has no access control between subscribers — anyone subscribed sees everything. Banned fields: email, phone, FIO, addresses, passwords, tokens, card numbers.

Findings on PII in `brief/data-bus/*.brief` → **P0** (PII leak to all subscribers).

### 3. Idempotent consumer handler

At-least-once delivery; duplicates are routine. Handler must produce the same effect on second delivery.

```go
// GOOD
func handler(ctx context.Context, msg *OrderCreated, meta databus.MessageMeta) databus.Result {
    exists, _ := db.OrderExists(ctx, msg.OrderId)
    if exists {
        return databus.Ack()
    }
    if err := processOrder(ctx, msg); err != nil {
        return databus.Defer(databus.DeferError(err))
    }
    return databus.Ack()
}

// BAD: unconditional INSERT → duplicates on retry
func handler(...) databus.Result {
    db.InsertOrder(ctx, msg)
    return databus.Ack()
}
```

## WARNING checks

### 4. ACK / NACK / DEFER / Skip — correct usage

| Result | When to use |
|---|---|
| `Ack` | success — consumer's copy is consumed |
| `Skip` | permanent bad message (validation failure, business rule rejected the event) |
| `Nack` | transient error, same consumer will retry; no new messages until ACK |
| `Defer` | move to next DLQ in chain; requires `dlq=[...]` in app.toml or behaves as NACK |

Common errors:
- `Nack` for business validation → infinite retry loop. Use `Skip`.
- `Ack` after error → silent message loss. Use `Defer`.
- `Defer` without `dlq=[...]` configured → falls back to NACK silently.

### 5. Nil message handling

Deserialization can fail (schema drift, bad publisher). Handler must check `if msg == nil` and not panic. Log with `meta.UnmarshalError()` + `meta.Payload()`, return `Skip`.

### 6. DLQ chain — escape hatch

```toml
[[data-bus]]
schema = "order.created"
dlq = ["1m", "5m", "30m"]
```

Handler should bound DLQ retries:

```go
if meta.DeferCount() > 50 {
    return databus.Skip()
}
```

DLQ topics need their own consumers (`ConsumerOfTopic`) to be read.

### 7. Deprecated `WithClientID()`

`databus.WithClientID(...)` is deprecated. Defer / DLQ reads / async k-branch routing don't work with it. Use named consumers in `app.toml` (`consumers = ["main", "analytics"]`) + `ConsumerNamedOf`.

### 8. Strict ordering not guaranteed

Partitioning means CREATE may arrive after UPDATE. Handlers must be order-tolerant (UPSERT, existence check), not assume sequence.

### 9. PGK (transactional outbox)

When to use: DB write + Data Bus publish must be atomic (otherwise state diverges from events).

When NOT to use: latency-critical paths (PGK adds ~500ms), publish without DB write, read-only flows.

Required pattern when used:

```go
tx, _ := db.BeginTx(ctx, nil)
ctx = pgkClient.WithTx(ctx, tx)  // mandatory
databusSDK.ProduceOrderCreated(ctx, events...)
err = tx.Commit()  // commit triggers send
```

Errors:
- `defer pgk_client.Flush(ctx)` then `tx.Commit()` — Flush will fire even on commit failure → ghost message.
- > 1000 messages in a single transaction (PgQ limit).
- Wrong client class: `NewQaaSClient` used with `databus.New()` → type error.
- Missing `WithTx` — produce silently uses non-transactional path.

When PGK is missing but needed (DB write + publish without PGK): **P1** (state/event divergence risk).
When PGK is configured but not needed (no DB write or publish unrelated to a transaction): **P3** info (unnecessary 500ms latency).

## Quick checklist

1. Message size ≤ 512 bytes JSON?
2. No PII in payload?
3. Handler idempotent? (exists-check / UPSERT / business ID dedup)
4. Ack/Skip/Nack/Defer used per the table above?
5. `if msg == nil` check present?
6. `DeferCount()` bound in DLQ-consuming handler?
7. No `WithClientID()`?
8. Order-tolerant?
9. PGK used iff DB write atomicity matters?
10. Flush after commit, not in defer?
