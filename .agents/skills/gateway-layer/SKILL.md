---
name: gateway-layer
description: Design, implement, review, or refactor Firestarter gateway contracts and adapters under `internal/domain/gateway` and `internal/infrastructure/gateway`. Use when integrating a provider from documentation, including typed config, Resty-based HTTP clients or provider SDKs, authentication or signing, transport DTOs, conversion boundaries, provider-aware errors, and focused gateway tests.
---

# Gateway Layer

## Purpose

Use this skill as the single source of truth for gateway contracts and external API implementations. It covers both design questions and implementation tasks such as `implement gateway @internal/infrastructure/gateway/... <docs link>`.

Gateways are infrastructure adapters behind domain-facing interfaces. Every direct HTTP integration in this repository uses Resty v2; a provider SDK is acceptable only when it owns the HTTP transport and the gateway does not make direct HTTP calls itself.

## Ownership

- `internal/domain/gateway` owns interfaces and domain-facing request/response contracts used by application services.
- `internal/infrastructure/gateway/<provider>` owns provider endpoints and DTOs, auth/signing, provider error classification, retry-safety and ambiguous-outcome policy, tracing evidence, and transport mapping. The `gateway` parent already establishes the adapter role, so do not repeat it in the package name.
- `internal/pkg/providerhttp` owns only confirmed provider-neutral gaps around Resty, such as a circuit-breaker `http.RoundTripper` or protocol parsing helpers.
- the singleton application `config` package owns each provider config type, loading, validation, and secret-safe credential representation.
- `internal/pkg/convert` owns reusable transport-neutral mapping only when conversion is shared outside one gateway method/package.

Do not put provider clients, Resty clients, JSON transport structs, provider auth fields, HTTP status codes, SDK response types, or provider-shaped responses into domain gateway contracts.

## Package Shape

Gateway implementation packages usually contain:

- `implementation.go` for shared client wiring, config usage, auth/signing hooks, and interface assertion
- one public method per file, named after the method in snake_case
- private request/response transport models colocated with the method that owns them
- focused tests for request construction, response mapping, auth helpers, and error handling

Keep provider behavior and ordinary Resty configuration explicit in each gateway constructor. Reuse `internal/pkg/providerhttp` only for confirmed provider-neutral gaps instead of wrapping Resty's existing API.

## Domain Contracts

Domain gateway contracts should be narrower than provider responses. Include only fields the application/domain currently needs.

Avoid:

- JSON tags in domain gateway DTOs
- Resty, HTTP, SDK, or provider client types in domain contracts
- provider-specific auth fields in application-facing requests
- copying entire provider responses upward to avoid mapping

Use typed enums for provider options with a documented closed set, and convert them to provider strings inside the gateway.

## Config

Define each provider configuration exactly once under the singleton application `config` package. A public gateway constructor takes no configuration argument and reads its section directly from `config.Instance()`; do not introduce a second gateway-local config struct, pass `config.<Provider>` into the public constructor, or add a field-by-field composition-root mapping for the same settings.

Use `github.com/go-playground/validator/v10` tags for ordinary validation such as required values, HTTPS URLs, numeric and duration bounds, and closed value sets. Keep supplemental validation code in `config/config.go` only for cross-field or security invariants that validator tags cannot express. Do not manually parse or revalidate a field in the gateway constructor after singleton config validation has accepted it. In particular, pass an already validated base URL directly to Resty rather than calling `url.Parse` or a project URL parser again.

Add only fields required by provider, worker, delivery, webhook-handler, or application behavior. When one provider section is shared by several components, keep the single decoded config model and let each consumer read only the fields it needs.

Typical fields:

- `BaseURL string`
- `Timeout time.Duration`
- `Debug bool`
- API key, bearer token, secret, private key, or network identifiers when required

Configuration loading is the authoritative validation boundary. Gateway constructors remain lightweight: bind `cfg := &config.Instance().<Provider>`, retain that pointer when runtime methods need provider settings, then construct the client from those already validated values without duplicating validation logic. Do not copy individual credentials, retry values, timeouts, network values, or other config fields into parallel `Implementation` fields. Keep only runtime state and genuinely derived values separate. Fx should register the zero-argument constructor directly rather than wrapping it in a provider whose only job is forwarding config.

Access secrets through an explicit conversion such as `string(i.cfg.APIKey)`. Do not call `Secret.String()` or `Secret.GoString()` for authentication because those methods intentionally return `[REDACTED]`.

Every HTTP gateway config must expose `Debug bool`, pass it directly to Resty with `SetDebug`, and set `Debug: true` in the repository example config. Use Resty's debug logger as-is because debug is restricted to local development. Do not install a custom logger or request/response log callbacks to sanitize debug output. Omit debug configuration entirely when credentials are embedded in request URLs or request/response bodies may contain secrets; Resty's raw debug output is unsafe at that boundary.

## HTTP / Resty Wiring

Construct and configure an ordinary `*resty.Client` explicitly in each direct HTTP gateway constructor. Gateway methods should retain Resty's normal request API (`client.R().SetContext(...).SetResult(...).Get/Post(...)`); do not replace it with gateway-local `do`, `doJSON`, or `perform` executors, a functional-options client builder, or a project request facade that hides Resty. Do not store, expose, inject, construct, or call a raw `*http.Client`; do not use `http.NewRequest` or `Client.Do` in gateway implementation code. Tests may replace Resty's `http.RoundTripper`.

Set an outgoing JSON content type with `SetHeader("Content-Type", "application/json")` on the Resty client or request. Do not use `Request.ForceContentType` for this purpose: in Resty v2 it overrides the response content type used for unmarshalling and does not set the request header. This distinction is mandatory when signing middleware serializes a structured body into `[]byte`, because Resty's later content detection otherwise classifies the bytes as `text/plain`.

Before adding resilience code or another dependency, inspect the exact pinned Resty version's API, implementation, tests, and existing repository references. Prefer Resty's own features where they satisfy the audited behavior:

- `SetRetryCount`, `AddRetryCondition`, and `AddRetryHook` for retries
- `SetRetryWaitTime` and `SetRetryMaxWaitTime` for Resty's capped exponential backoff with jitter
- `SetRetryAfter` for provider `Retry-After` behavior
- `SetRateLimiter` for fail-fast `Allow()` limiting

Add a narrow extension only for a confirmed gap. For example, use an `OnBeforeRequest` hook with `rate.Limiter.Wait(ctx)` when calls must wait rather than fail fast, and use a small `http.RoundTripper` decorator backed by `sony/gobreaker/v2` when circuit breaking is required. Do not introduce `retry-go`, custom jitter, or a custom request-execution engine merely to reproduce Resty behavior.

Keep exactly one retry owner. Audit Resty's transport-error defaults and client-wide retry count before enabling retries: safe reads may retry, while writes default to one attempt unless the provider contract supplies an idempotency mechanism. Use separate appropriately configured clients or an explicit request policy when necessary to prevent unsafe writes from inheriting retries. Preserve an explicit unknown-result outcome when a write may have reached the provider.

Typical constructor:

```go
type Implementation struct {
    cfg    *config.Provider
    client *resty.Client
}

func NewImplementation() *Implementation {
    cfg := &config.Instance().Provider
    client := resty.New().
        SetBaseURL(cfg.BaseURL).
        SetTimeout(cfg.Timeout).
        SetResponseBodyLimit(int(cfg.MaxResponseBytes)).
        SetDebug(cfg.Debug)

    return &Implementation{
        cfg:    cfg,
        client: client,
    }
}
```

If provider docs require header-based API keys, bearer tokens, timestamps, or request signatures, attach them once with a gateway-local Resty `OnBeforeRequest` middleware. Do not set authentication headers in individual methods or rely on each request builder to remember them. The middleware must derive dynamic signatures from the final method, path, query, and serialized body required by the provider contract, and return only secret-safe errors. Keep credentials in the closure or gateway implementation, and add focused tests proving that the middleware authenticates every relevant request. Tokens that are contractually part of the URL path and provider SDK-managed authentication are explicit exceptions.

For deterministic tests, construct the gateway normally, replace `implementation.cfg` with a pointer to a test-owned copy when provider values matter, and replace the Resty client's transport with `SetTransport`. Do not mutate the singleton, add a configuration parameter, add a second constructor, or add gateway-local transport machinery solely for testing.

## Transport Models

Keep endpoint constants, request payload structs, response payload structs, and mapping helpers in the method file that uses them unless multiple methods genuinely share the same transport contract.

Use private transport DTOs with exact provider JSON field names. Do not reuse domain DTOs as raw HTTP payloads, and do not add provider tags to domain models.

Model documented scalar wire fields with their direct Go types. Do not introduce a named microtype and custom JSON unmarshaler merely to accept undocumented alternate representations. Add custom scalar decoding only when provider documentation or verified payloads establish a real polymorphic wire contract.

Use standard parsing functions directly for scalar string inputs; do not precede `strconv.ParseInt`, `strconv.ParseUint`, or equivalent parsers with a hand-written character classifier that repeats their work.

For provider monetary values represented as decimal JSON numbers or strings, use `github.com/shopspring/decimal` at the transport boundary instead of writing a decimal parser or performing decimal arithmetic manually. Never route money through `float32` or `float64`. Before mapping a decimal into the domain's integer minor-unit representation, reject absent or negative values when disallowed, require an exact currency-scale conversion without rounding or truncation, and verify the result fits the destination integer type. Keep the domain representation in integer minor units unless its approved contract explicitly requires arbitrary-precision decimals.

Preferred helper names:

- `new<Method>Request(...)`
- `(r <method>Response) toDomain()`
- `(r <method>Response) toDTO()` when the domain gateway contract returns a DTO

Use `internal/pkg/convert` only for reusable, provider-neutral mapping.

## Implementation Workflow

When implementing a gateway from docs:

1. Inspect the target domain gateway interface and infrastructure package.
2. Inspect singleton application config, constructor wiring, and adjacent gateways for local style.
3. Inspect the provider docs before editing.
4. Identify endpoint path, method, params, body, status codes, response shape, and auth/signing requirements.
5. Add or adjust the single provider type and validator tags in `config/config.go`; add supplemental centralized validation only when tags cannot express the invariant.
6. Construct and configure Resty directly in the gateway constructor, using its native retry, jitter, `Retry-After`, limiter, and debug facilities where they meet the required semantics; add only narrow helpers or transport decorators for confirmed gaps such as blocking rate limiting or circuit breaking. Attach one provider-local `OnBeforeRequest` auth/signing middleware.
7. Add private transport request/response models near each method.
8. Implement the provider call through Resty's direct request API with `SetContext(ctx)`.
9. Record bounded, secret-safe response evidence immediately after Resty returns and before status/error classification, so non-success responses retain status, provider code, response size, and attempt information.
10. Map successful provider responses into domain-facing DTOs/entities and replace preliminary evidence with richer domain evidence when useful.
11. Preserve local tracing and error conventions.
12. Add focused tests that do not require live provider access unless the repo already has explicit env-gated integration tests.
13. Audit private DTOs and helper state for fields that are decoded or assigned but never read. Compiler and ordinary unused-code checks may not report write-only fields; remove them and any types made unreachable.

If docs are ambiguous, inspect adjacent code first, choose the safest endpoint only when defensible, and state the ambiguity.

## Method Shape

Typical HTTP method boundary:

```go
func (i *Implementation) GetSomething(ctx context.Context, req gateway.GetSomethingRequest) (*gateway.GetSomethingResponse, error) {
    ctx, span := prospan.Start(ctx)
    defer span.End()

    var body getSomethingResponse
    resp, err := i.client.R().
        SetContext(ctx).
        SetQueryParams(newGetSomethingQuery(req)).
        SetResult(&body).
        Get("/something")
    if err != nil {
        return nil, span.Err(errors.Join(&proterror.Unavailable{}, err))
    }

    if err = classifyProviderStatus(resp.StatusCode()); err != nil {
        return nil, span.Err(err)
    }

    return body.toDomain(), nil
}
```

Treat every documented 2xx status deliberately. Map authentication, authorization, invalid-request, rate-limit, and provider 5xx responses to the narrowest project error classification. Never place an unbounded raw provider body into an error or span; decode a bounded error DTO and retain only safe diagnostic fields.

Retry only documented transient failures and only when the operation is idempotent or carries a provider-supported idempotency key. Do not add automatic retries to side-effecting requests by default.

After an ambiguous side-effecting create, recover by a stable provider idempotency key or request identity before issuing another create. If the provider offers lookup by a caller-supplied name, persist and reuse that name. Never make “provider ID missing” itself the condition for repeating a paid request.

Gateway recovery does not replace an application-level dispatch claim: two workers can both observe “not found” and race into the create call. The aggregate must durably elect one dispatcher before the recovery-then-create sequence.

Bind lookup results to all immutable request facts available at the boundary: provider request identity, wallet, network, resource or asset type, requested quantity, and relevant period. Treat mismatches and multiple matches as failed preconditions, not successful reconciliation.

When an operator supplies provider-verified completion facts for a security or financial reconciliation, require the bounded evidence digest mandated by the domain contract. Authentication proves who submitted the result; it does not replace evidence binding.

Provider-declared success is insufficient when the business outcome is independently observable. For delegated chain resources, query the chain and expose the available energy or bandwidth; the application may finalize only after the required quantity is visible on-chain.

A provider `PENDING` or 5xx response to a paid create is not retryable merely because it is non-terminal. If the provider exposes no lookup or idempotency key for that created object, return an ambiguous disposition and prohibit automatic POST retries. Reserve retryable status for pre-dispatch failures or safe reads such as GET polling by a persisted provider ID.

Represent definitive provider rejection as a typed domain-facing disposition distinct from transport ambiguity. The application layer must be able to terminally fail the aggregate instead of endlessly retrying the same rejected request.

## Testing

Require deterministic tests that validate:

- request DTO mapping
- response-to-domain mapping
- status/error handling
- auth/signing helper behavior
- client-level `OnBeforeRequest` authentication across every relevant gateway operation
- endpoint/path/query/body construction through `httptest` when needed
- exact outbound `Content-Type` when required by the provider contract
- config-driven Resty debug wiring through native `SetDebug`, unless the gateway omits debug because URLs or bodies contain secrets
- every supported success status and representative 4xx, rate-limit, and 5xx mappings
- response size or malformed-body handling when the gateway decodes arbitrary provider data

Do not add tests that call real external endpoints by default. Use live calls only when the repository already has env-gated integration tests or the user explicitly asks for them.

## Boundary Smells

Pause and re-check the design if gateway code:

- leaks provider JSON DTOs into `internal/domain/gateway`
- makes application services depend on concrete gateway packages
- hardcodes config values in methods
- duplicates a provider configuration struct inside the gateway or maps an identical field list in `cmd`
- accepts configuration in a public gateway constructor instead of reading the singleton section directly
- wraps a zero-argument gateway constructor in an Fx provider that only forwards configuration
- mirrors individual singleton config values as implementation fields instead of retaining one config-section pointer
- calls a redacting `Secret.String()` or `Secret.GoString()` method when the real credential value is required
- parses or revalidates already validated singleton configuration in the constructor
- implements required, URL, range, or enum checks manually when existing `validator/v10` tags express them
- keeps a raw `*http.Client`, calls `http.NewRequest` or `Client.Do`, or accepts a raw HTTP client for test injection
- uses `Request.ForceContentType` to configure an outgoing request instead of setting the `Content-Type` header
- hides ordinary Resty configuration behind a project functional-options client builder
- implements retry loops, sleep, backoff, jitter, `Retry-After`, circuit breakers, or rate limiters inside a provider gateway
- adds a request-execution wrapper or dependency without first auditing the pinned Resty API, source, tests, and repository usage
- duplicates Resty's retry, capped exponential jitter, `Retry-After`, or limiter capabilities without a documented semantic gap
- combines multiple retry owners or retries a write without documented idempotency
- omits the config-driven Resty debug option without a URL-credential or sensitive-body exception, or wraps Resty's local-development debug logger with custom sanitization
- sets API keys, bearer tokens, timestamps, or signatures in individual request methods instead of one client-level `OnBeforeRequest` middleware
- concatenates URLs unsafely instead of resolving paths and query parameters with URL helpers
- invents auth behavior not supported by docs
- implements custom decimal parsing or decimal arithmetic for provider monetary values instead of using `shopspring/decimal` plus explicit scale and range validation
- routes monetary values through binary floating-point or silently rounds/truncates them while mapping to integer minor units
- mirrors provider responses into domain contracts without a current business need
