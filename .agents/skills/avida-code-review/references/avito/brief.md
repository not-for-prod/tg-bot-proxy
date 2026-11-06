# Brief / RPC Contracts (Avito PaaS)

**Activates when**: any `brief/rpc/*.brief`, `brief/rpc/clients/*.brief`, `brief/rpc/service/*.brief` in diff; or `[[dependencies]]` with `brief_rpc` in `app.toml`.

**Used by**: `architect-reviewer`, `fullstack-developer`.

Not for writing contracts — for reviewing them. Universal `avito lint` covers syntax; this covers semantics.

## CRITICAL checks

### 1. Backward compatibility — server side

**Safe without client migration:**
- Make an incoming field optional (`name string` → `name *string`).
- Add an optional field to incoming.
- Make an outgoing field required (`data *Data` → `data Data`).
- Add any field to outgoing.
- Remove unused method or field.
- Add `idempotent` flag.

**Breaks all clients (P0/P1):**
- Make incoming field required (was optional, now required) → **P0** if any client exists.
- Make outgoing field optional (was required, now optional) → **P0**.
- Remove a required outgoing field → **P0**.
- Remove `idempotent` flag on a method → **P1** (clients may have removed their own retry).
- Change field type → **P0**.

Safe migration for new required incoming field:
1. Add as optional `*Type`.
2. All clients add and start sending.
3. Server flips to required.

## WARNING checks

### 2. Naming conventions

| Element | Convention | Examples |
|---|---|---|
| RPC methods | lowerCamelCase | `getUser`, `createOrder` |
| Messages | UpperCamelCase, In/Out suffix | `SearchIn`, `SearchOut` |
| Fields | lowerCamelCase | `userEmail`, `itemCount` |
| typed const | UpperCamelCase name, UPPER_CASE values | `Status { ACTIVE = 1 }` |

Bad:
- `rpc "/some/path"` (legacy path style) → P1
- `rpc MyMethod` (UpperCamel) → P2
- `message search_in` (snake_case) → P2
- `Query string` (UpperCamel field) → P2
- Legacy `const` instead of `typed const` → P3

### 3. Idempotency declaration

```brief
idempotent rpc put(PutIn) PutOut
```

Findings:
- Mutating method (create/update/delete) without `idempotent` → P2 (platform won't transparent-retry).
- `idempotent` on streaming RPC → P1 (invalid combination).
- `idempotent` on a method whose handler is **not** actually idempotent → P1 (false promise; retries cause duplicates).

### 4. Timeouts

Default brief_rpc timeout is **1 second** — almost always wrong.

```toml
[[dependencies]]
name = "atlas"
brief_rpc = [
    { name = "getServiceFeatures", timeout = "100ms" },
    { name = "getServiceApptoml", timeout = "500ms", connection_timeout = "50ms" },
]
brief_rpc_default = { timeout = "10ms" }
```

Findings:
- New RPC client used in code without timeout in `app.toml` (except streaming) → P1.
- Timeout incompatible with the target service's NFR (e.g. caller timeout 100ms when target SLO promises 500ms p95) → P2.

### 5. Streaming RPC

Beta; Go only. Types: `(stream X) Y`, `X (stream Y)`, `(stream X) (stream Y)`.

Findings:
- `idempotent rpc upload (stream FileChunk) UploadResult` → P1 (invalid).
- Timeout set in `app.toml` for streaming RPC → P2 (no overall timeout in streaming; control must be in business logic).

### 6. Validation vs ServerError

- `ValidationError`: client violated the contract (bad format, unknown ID, business rule failure).
- `ServerError`: server couldn't fulfil (DB down, dependency timeout, internal bug).
- **Business outcomes** ("insufficient balance", "user blocked") → return in response DTO, not as exception:

```brief
rpc buyItem(BuyItemIn) BuyItemOut

message BuyItemOut {
    ok     bool
    error *Error
}

message Error {
    insufficientBalance *InsufficientBalanceError
    userBlocked         *BlockedUserError
}
```

Business outcome thrown as `ServerError` / `ValidationError` → P2.

### 7. Antipatterns

| Pattern | Severity | Notes |
|---|---|---|
| `message In {}` (empty message) | P3 | not banned but discouraged |
| Recursive struct without optional pointer (`child Node` instead of `child *Node`) | P1 | recursion validation error |
| `map[int]*int` | P1 | optional values in map not supported; use wrapper message |
| `[][]int` | P1 | 2D arrays not supported; use wrapper |
| `bytes` field without base64 handling note (Python) | P2 | Python doesn't decode automatically |
| `datetime` field — library version supports millisecond precision | P3 | check version |

### 8. Multi-file `service.brief`

Either single `service.brief` or directory `service/` — never both. Each file in `service/` carries `service "name"` header and contributes to one cross-file namespace; duplicate message/field names across files → P1.

### 9. Deprecation

Old: `field int \`Deprecated: use newField\``.
New: `//brief:deprecated use newField\nfield int`.

Deprecated method still used by new clients in this PR → P2.

### 10. Annotations

```brief
//brief:tag user
rpc getUser(GetUserIn) GetUserOut

//brief:sensitive EMAIL, COMPANY_NAME
userData string

//brief:nonsensitive
rpc toString(ToStringIn) ToStringOut
```

Findings:
- `// brief:tag` (space after `//`) → P3 (annotation will be ignored).
- Annotation on the wrong line (to the right instead of above) → P3.
- `//brief:nonsensitive` on a method whose In/Out contains `//brief:sensitive` fields → P1 (contradictory annotations).

## Quick checklist

1. Backward compat: no new required In fields, no removed required Out fields, no type changes?
2. Naming follows the conventions table?
3. Mutating methods marked `idempotent`?
4. `idempotent` not on streaming?
5. Every RPC client has explicit timeout in `app.toml`?
6. Streaming RPC has no `app.toml` timeout?
7. Business outcomes in response DTO, not as exception?
8. No recursive required fields / unsupported types?
9. Single `service.brief` xor `service/` directory?
10. `//brief:` annotations on correct line, no space after `//`?
