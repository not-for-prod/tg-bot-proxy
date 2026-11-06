# Queues / QaaS (Avito PaaS)

**Activates when**: `[[queues]]` in `app.toml`, `brief/qaas/*.brief`, or Go/Python consumer/producer code using `queues.*` SDK.

**Used by**: `fullstack-developer`.

QaaS = intra-service messaging. One consumer per message (no fan-out). Distinct from Data Bus.

## CRITICAL checks

### 1. Idempotent handler (at-least-once delivery)

Duplicates are routine (network glitches, restart-before-ack, QaaS hiccups).

```go
// GOOD: check + idempotent op
func handler(ctx context.Context, msg *OrderCreated, meta queues.MessageMeta) queues.SingleResult {
    exists, _ := db.OrderExists(ctx, msg.OrderId)
    if exists {
        return queues.Ack()
    }
    if err := processOrder(ctx, msg); err != nil {
        return queues.Defer(queues.DeferError(err))
    }
    return queues.Ack()
}

// or via middleware
handler := queues.DeduplicationSingleMW[*MyMessage](opts)(myHandler)
```

Non-idempotent handler that performs writes → **P0** on the money path, **P1** elsewhere.

## WARNING checks

### 2. ACK / NACK / DEFER / Skip semantics

| Result | Use for |
|---|---|
| `Ack` | success |
| `Skip` | permanent bad message — won't retry |
| `Nack` | transient error, same consumer retries; **blocks new messages until success** |
| `Defer` | move to DLQ chain; requires `dlq=[...]` |

Errors:
- `Nack` for business validation (`balance < amount`) → infinite loop. Use `Skip`.
- `Ack` after error → message loss. Use `Defer`.
- `Defer` without DLQ configured → silently behaves as `Nack`.

### 3. DLQ chain

```toml
[[queues]]
schema = "order.created"
dlq = ["1m", "5m", "30m", "1h"]
```

Handler must bound DLQ retries:

```go
if meta.DeferCount() > 50 {
    return queues.Skip()
}
if meta.DeferInCurrentCount() >= 5 {
    return queues.Defer(queues.DeferReason("max retries in current DLQ"))
}
```

Or via middleware: `queues.RepeatInCurrentSingleMW[*MyMessage](...WithMaxCount(5))(handler)`.

### 4. Nil message handling

```go
if msg == nil {
    log.Warn("failed to unmarshal", "payload", string(meta.Payload()))
    return queues.Skip()
}
```

Missing nil check → P1 (panic on bad payload).

### 5. Order not guaranteed

Partitioning + Nack/Defer means CREATE may arrive after UPDATE. Handler must be order-tolerant — UPSERT, existence checks, timestamp / version comparison.

### 6. Batch handling

```go
queues.SubscriberOf(client, queues.ToHandlerBatchResultFunc(
    func(messages []*MyMessage, metas []queues.MessageMeta) queues.BatchResult {
        // ...
    },
))
```

Error in one message must not poison the whole batch. Use partial `Acks(n, WithDefer(...))`.

### 7. Subscriber lifecycle

```go
// BAD: blocks main
consumer.Run(ctx)
http.ListenAndServe(...)  // never reached

// GOOD: separate goroutine with panic recovery
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Error("consumer panic", "error", r)
        }
    }()
    consumer.Run(ctx)
}()
```

### 8. Context propagation

`meta.Context()` carries tracing and x-route — pass it forward, don't reset to `context.Background()`.

### 9. Reader mode caveat

Reader mode (no ACK, no offset, broadcast to all readers) is **signals only** — cache invalidations, notifications. Using it for business logic loses messages → P1.

### 10. PGK (transactional outbox)

Same rules as Data Bus PGK, but use `pgk_client.NewQaaSClient` and `qaas.New`. Mixing clients (Data Bus client with QaaS SDK) is a type error.

```go
pgkClient := pgk_client.NewQaaSClient(...)
qaasSDK, _ := qaas.New(pgkClient)

tx, _ := db.BeginTx(ctx, nil)
ctx = pgkClient.WithTx(ctx, tx)
qaasSDK.PublishOrderTask(ctx, msg)
err = tx.Commit()
```

PGK pitfalls (same as Data Bus): `Flush` in defer, missing `WithTx`, > 1000 messages per transaction, wrong client.

When PGK missing but DB-write + publish atomicity matters → P1.

## Quick checklist

1. Handler idempotent?
2. Ack/Skip/Nack/Defer per the table?
3. DeferCount / DeferInCurrentCount bounded?
4. `if msg == nil` check?
5. Handler order-tolerant?
6. Batch handler tolerates per-message errors?
7. `consumer.Run` in separate goroutine with recover?
8. `meta.Context()` propagated?
9. Reader mode not used for business logic?
10. PGK: WithTx + Flush after commit + ≤1000 msgs?
