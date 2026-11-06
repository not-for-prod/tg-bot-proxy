# Sensitive Data in Logs (Avito PaaS)

**Activates when**: any `*.brief` file in repo contains `//brief:sensitive` annotations.

**Used by**: `security-auditor`.

Goal: catch PII leaks where sensitive fields declared in Brief contracts end up in logs in cleartext.

## Workflow

### Step 1. Collect sensitive fields from brief

1. Enumerate `*.brief` files in `brief/`, **including** `brief/rpc/clients/` (responses from other services arrive here too).
2. Extract every `//brief:sensitive <TAG>` annotation. Note: field name, type, tag (EMAIL, PHONE, FIO, USER_ID, ...), parent message name, file path.

Example:

```brief
message User {
    //brief:sensitive USER_ID
    id     int
    //brief:sensitive EMAIL
    email  string
    //brief:sensitive PHONE
    phone *string
    status string  // not sensitive
}
```

→ Sensitive fields: `id` (USER_ID), `email` (EMAIL), `phone` (PHONE).

For the authoritative list of allowed `sensitive` tags and their severity, fetch via:

```bash
avito rpc call -s service-schema -e prod listAnnotationValues '{"name": "sensitive"}'
```

### Step 2. Find sensitive fields in log calls

Search Go code in `internal/`, `cmd/` for these patterns by field name:

```
log\.(Info|Warn|Error|Debug|WithField|WithFields).*\b<fieldName>\b
fmt\.(Sprintf|Printf|Fprintf).*\b<fieldName>\b
log\.(Print|Printf|Println).*\b<fieldName>\b
logger\.\w+.*\b<fieldName>\b
```

Search both by brief field name (`email`, `phone`) and DTO accessor (`req.Email`, `user.Phone`, `in.Name`).

### What IS a leak

| Pattern | Severity |
|---|---|
| `log.WithField(ctx, "email", user.Email)` | **P0** (cleartext PII) |
| `fmt.Sprintf("user %s phone %s", name, phone)` then logged | **P0** |
| `log.Info(ctx, fmt.Sprintf("got request: %+v", req))` where `req` has sensitive fields | **P0** (whole struct dump) |
| `log.WithField(ctx, "user", fmt.Sprintf("%+v", user))` | **P0** |

### What is NOT a leak

- Field passed to business logic / RPC response (that's the contract).
- Already masked: `mask.MaskEmail(email)`, `sanitize.MaskPhone(phone)`.
- Existence/length only: `"has_email": email != ""`, `"phone_len": len(phone)`.
- Hash: `"email_hash": hash(email)`.

### Step 3. Find existing sanitize utilities

```
Glob: **/mask*.go, **/sanitize*.go, **/pii*.go
Grep: func Mask, func Sanitize, func Redact
```

Typical paths: `internal/utils/mask/`, `internal/pkg/sanitize/`, `internal/common/mask/`, `pkg/utils/`.

### Step 4. Suggest fix

- If sanitize utils exist → reuse: `log.WithField(ctx, "phone", mask.MaskPhoneNumber(user.Phone))`.
- If not → recommend creating `internal/utils/sanitize/sanitize.go` with `MaskPhone`, `MaskEmail`, `MaskName`.
- For whole-struct logging: switch to per-field logging, or add `LogValue()` method that masks.

### Step 5. Whole-DTO logging

Special focus — `%+v` and `%v` on DTOs that contain sensitive fields:

```go
// CRITICAL
log.WithField(ctx, "request", req)
log.Info(ctx, fmt.Sprintf("got request: %+v", in))
```

Fix:

```go
// Option A: log only non-sensitive fields explicitly
log.WithFields(ctx, map[string]any{
    "order_id": req.OrderID,
    "status":   req.Status,
})

// Option B: implement LogValue / String on the DTO
func (u User) LogValue() slog.Value {
    return slog.GroupValue(
        slog.Int("id", u.ID),
        slog.String("email", sanitize.MaskEmail(u.Email)),
    )
}
```

## Report shape

```markdown
### Sensitive fields from brief
| Field | Tag | Message | Brief file |
|---|---|---|---|
| email | EMAIL | UserInfo | brief/api.brief |
| phone | PHONE | UserInfo | brief/api.brief |

### Existing sanitize utilities
- internal/utils/mask/mask.go — MaskPhone, MaskEmail
   (or "none found")

### Findings (P0)
- handler.go:42 — `email` (EMAIL) logged cleartext via `log.WithField(ctx, "email", user.Email)`
  Fix: `log.WithField(ctx, "email", mask.MaskEmail(user.Email))`
```

## Quick checklist

1. Brief sensitive fields enumerated?
2. Log calls grepped for each field name + DTO accessor?
3. Whole-DTO `%+v` / `%v` logs flagged for DTOs with sensitive fields?
4. Sanitize utility existence verified before suggesting creation?
5. Each finding pairs leak location with concrete fix?
