---
name: method-per-file
description: Apply Firestarter's focused-file convention to public application-service, repository, gateway, and delivery-worker methods or constructors. Use when adding, renaming, or refactoring public behavior so file names stay synchronized and method-owned DTOs, constructors, conversions, and behavior remain in one focused file; do not use it for domain aggregate methods.
---

# Method Per File

## Overview

Use one file per public application-service or adapter method. Keep method-local DTOs and mapping helpers in the same file as the method that owns them.

Derive the file name mechanically from the public Go method name:

1. take the CamelCase/PascalCase method identifier
2. convert its word boundaries to lowercase snake_case
3. append `.go`

Examples:

- `ListPositions(...)` -> `list_positions.go`
- `GetBalanceAllowance(...)` -> `get_balance_allowance.go`
- `BroadcastSignedTransfer(...)` -> `broadcast_signed_transfer.go`
- `GetAMLCheck(...)` -> `get_aml_check.go`

Treat a Go initialism as one lowercase word: `AML` becomes `aml`, `ID` becomes `id`, and `URL` becomes `url`.

Do not create broad noun files such as `positions.go` when the public method is `ListPositions(...)`.

During review, inspect policy, executor, handler, and similarly broad files for public methods hidden behind the type or role name. Constructors and dependency structs may remain in those files, but each public application-service, repository, or gateway operation belongs in its own exact snake-case method file.

When renaming a public method, rename its owning file in the same change. Search for the old method and old snake-case file name afterward; neither may remain unless it represents a separate current concept.

This skill applies to:

- `internal/application/service/...`
- `internal/infrastructure/repository/...`
- `internal/infrastructure/gateway/...`
- `internal/delivery/worker/...`
- other adapter packages with boundary DTOs and domain conversion

Do not split each domain aggregate method into a separate file; domain aggregate organization follows the domain-entity and aggregate-boundary rules. Prefer existing package conventions when they are already clear, but use this convention for new application/adapter files and focused refactors.

## File Order

Order method files from local models to behavior:

1. request DTOs used at the adapter boundary
2. request DTO constructors, such as `newXRequest(...)`
3. response DTOs or row DTOs
4. response/row conversion methods, such as `(r xResponse) toDomain()`
5. xo/domain converters for repository files
6. the public repository/gateway method itself

The method should usually be last, so readers see the local data shapes and conversions before the orchestration.

Application-service files usually have no local transport DTOs. Keep the public command or query method in its matching snake-case file and leave shared constructor/dependency wiring in `implementation.go`.

Every private helper called by only one public method belongs in that public
method's file. This includes request or row mapping, payload encoding and
decoding, validation, error construction, and method-specific query
composition. Do not create a broad noun file merely to hold helpers, such as
`operation_payload.go`, when each helper has a clear public-method owner. If a
broad helper file contains behavior owned by different public methods, split
the behavior into the respective method files. Keep a shared helper separate
only when multiple public methods directly call the same non-trivial behavior
and the extraction passes the `function-decomposition` duplication gate.

Tests for a method-owned helper belong in the matching `<method>_test.go` file,
not a standalone helper-noun test file.

## Worker Families

Organize delivery workers by one operational family per file. Closely related
variants may share their family file with the private constructor or job logic
they use: external and internal pollers belong in `poller.go`, inbox processors
belong in `inbox.go`, and an outbox publisher belongs with its dispatch logic in
`outbox.go`. Do not collect unrelated worker constructors in `workers.go` merely
because they all satisfy the same `worker.Worker` interface.

## Gateway DTOs

For gateway methods:

- use private request/response DTOs with `json` tags for external API payloads
- keep DTOs method-local unless they are truly shared by multiple methods
- add `newXRequest(...)` when mapping from domain request to gateway DTO is non-trivial or consistency is valuable
- use `(r xResponse) toDomain()` when converting external responses into domain responses/entities
- do not leak gateway DTOs into domain, application, or service packages
- do not mirror a provider response into a domain-facing contract just because the provider returns many fields; map only the fields the project currently needs

## Repository DTOs

For repository methods:

- use xo models directly when the query shape matches a generated xo model
- use private row DTOs only for custom query shapes, joins, aggregates, computed columns, or projections that cannot be represented by one xo table model
- treat missing xo models or helpers as a schema/generation problem, not as a reason to create duplicate table-shaped DTOs
- add `db` tags only when the scanner uses tags, such as struct-by-name scanning
- do not add `db` tags for plain `rows.Scan(...)`; they do nothing there
- keep SQL, xo types, row DTOs, and persistence conversion in infrastructure

## Converter Naming

For repository files using xo models, entity conversion belongs in `internal/pkg/convert`.

Use free functions there:

- `convert.<EntityName>FromXO(...)`
- `convert.<EntityName>ToXO(...)`

Use this naming when converting between a domain entity and an xo-generated persistence model. Do not keep entity-to-xo or xo-to-entity converters package-local in repository packages.

For custom response or row DTOs, prefer methods:

- `(r xResponse) toDomain()`
- `(r xRow) toDomain()`

Use package-local helpers only for DTOs, custom response shapes, custom row projections, or gateway payload mapping.

## Boundary Rules

- DTOs belong to the adapter that owns the transport or persistence shape.
- Domain entities and domain events should not gain JSON, DB, topic, header, or provider-specific fields only to satisfy an adapter.
- Infrastructure may convert technical DTOs into real domain models.
- Do not invent adapter-local pseudo-domain types that escape through domain-facing interfaces.
- During review, search every private helper's production callers and move single-owner helpers into the caller's method file.
