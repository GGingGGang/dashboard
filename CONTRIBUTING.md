# Contributing an adapter

The application owns polling, credentials, local persistence, and UI state. Each adapter translates an upstream API into the models in `internal/platform/model.go`.

## Existing contract

Every provider implements `Check(context.Context)` and `Discover(context.Context)`. Implement one of:

- `CI`: paged `Builds`, individual `Build`, and `Queue`. The current archive contract uses numeric build identifiers and millisecond timestamps. Return normalized states (`RUNNING`, `SUCCESS`, `FAILURE`, `UNSTABLE`, `ABORTED`, `NOT_BUILT`, or `UNKNOWN`) and preserve the upstream state in `RawStatus`.
- `CD`: `Deployments`. Keep desired/observed synchronization separate from runtime health. Missing upstream fields remain unknown; they are never fabricated.
- `Monitoring`: `Query`, supporting the declared instant/range capabilities. Numeric gaps use a nil value. The current editor and presets are PromQL-specific.

For CI, `Builds` receives an offset, returns up to 100 newest-first rows and indicates whether there may be another page. Populate connection ID, stable job ID, start time, observed time, and source link. A missing individual build returns an `APIError` with status 404 so the collector can retain it as `UNCONFIRMED`. Upstreams with cursor-only pagination need adapter-local translation; do not change the archive identity silently.

## Add an implementation

1. Add an adapter under `internal/platform`. Use the shared HTTP helper for GET requests, authentication, verified TLS, cancellation, timeouts, bounded responses, and sanitized errors. Construct API paths from validated identifiers; never request an arbitrary URL supplied by upstream JSON.
2. Add one entry to `registry` in `internal/platform/http.go`: constructor, provider name, category, default authentication, and supported capability names. Validation and provider listings use this registry. The frontend builds the provider selector and chooses existing views from these capabilities.
3. Add `httptest` fixtures for successful reads, authentication failures, missing data, unknown states, cancellation, and the relevant pagination/identifier edge cases. Tests must not require personal infrastructure or real tokens.
4. For CI/CD, test the collector and archive behavior with the new adapter. For monitoring, prove the documented result limits and empty/stale/error handling. Add a regression case when changing semantics.
5. Document supported API versions, permissions, configuration, unsupported fields, and any external prerequisites. Distinguish fixture tests from real integration results.
6. Run `scripts/test.ps1` and `scripts/build.ps1`. Manually inspect a desktop demo after changes to the Wails bridge or window lifecycle.

Capabilities currently consumed by the UI include `builds`, `deployments`, and `promql`. Other declared names (`queue`, `history`, `sync`, `health`, `instant`, `range`, `rules`) document the supported contract. A provider must implement the complete selected interface; advertising a partial interface does not make unsupported calls work. If a new provider cannot support a complete interface, explicitly evolve the contract and UI together.

Do not add provider-specific polling to the frontend, bake in cluster addresses, disable certificate validation, log credentials, or put secrets into fixtures. Basic and bearer authentication already work for all three current adapters. OAuth/device login and token refresh are not implemented.

## Evidence for review

Include the problem, before/after behavior, a small reproducible fixture, tests run, and remaining limits. Source loss, duplicate observations, number reuse, and failed queries are more useful evidence than screenshots alone. The current results are recorded in `VERIFICATION.md`; update that record only for checks actually performed.
