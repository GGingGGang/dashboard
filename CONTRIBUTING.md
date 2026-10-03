# Contributing an adapter

The application owns polling, credentials, local persistence, and UI state. Each adapter translates an upstream API into the models in `internal/platform/model.go`.

## Existing contract

Every provider implements `Check(context.Context)` and `Discover(context.Context)`. Compose any of these independent capabilities:

- `BuildSource` (`builds`): paged `Builds` and individual `Build`. The current archive contract uses numeric build identifiers and millisecond timestamps. Return normalized states (`RUNNING`, `SUCCESS`, `FAILURE`, `UNSTABLE`, `ABORTED`, `NOT_BUILT`, or `UNKNOWN`) and preserve the upstream state in `RawStatus`.
- `QueueSource` (`queue`): current queue entries. This is optional for build providers.
- `DeploymentSource` (`deployments`): `Deployments`. Keep desired/observed synchronization separate from runtime health. Missing upstream fields remain unknown; they are never fabricated.
- `MetricSource` (`metrics`): `Query`, with instant/range numeric series. Numeric gaps use a nil value. Set `ProviderInfo.Query` with the query language, default expression, and presets; the frontend no longer assumes PromQL. The current metric editor expects both query modes.

`CI`, `CD`, and `Monitoring` remain compatibility names. A combined platform can implement multiple interfaces. Use `Target.Capabilities` to distinguish build and deployment discovery results; omit it for legacy single-purpose targets. See [ARCHITECTURE.md](ARCHITECTURE.md) for data lifetimes and remaining archive limitations.

For CI, `Builds` receives an offset, returns up to 100 newest-first rows and indicates whether there may be another page. Populate connection ID, stable job ID, start time, observed time, and source link. A missing individual build returns an `APIError` with status 404 so the collector can retain it as `UNCONFIRMED`. The current contract requires offset pages. Cursor-only APIs need an explicit compatibility design or a contract change; do not fabricate numeric build IDs or silently change archive identity.

## Add an implementation

1. Add `internal/providers/<name>/<name>.go` and keep its `*_test.go` fixtures beside it. Implement the relevant interfaces from `internal/platform`. Use `internal/providers/internal/httpapi` for read requests, authentication, verified TLS, cancellation, timeouts, bounded responses, and sanitized errors. The sole current POST is the explicitly selected Argo CD session login. Construct API paths from validated identifiers; never request an arbitrary URL supplied by upstream JSON.
2. Export `Definition() platform.Definition` containing `Info` and a `Create(Connection, secret)` factory. Include an optional `Validate(Connection) error` for provider-specific constraints checked before saving and construction. Add its import and `Definition()` to `internal/providers/builtin.go`; custom builds can instead pass it to `NewApp(demoMode, definition)` in `main.go`. Include name, category, default authentication, supported `AuthMethods`, capabilities, `PollSeconds`, and query metadata when applicable. No collector or storage switch needs a new provider name. The registry rejects duplicate names and mismatches between interfaces and declared capabilities.
3. Add `httptest` fixtures for successful reads, authentication failures, missing data, unknown states, cancellation, and the relevant pagination/identifier edge cases. Tests must not require personal infrastructure or real tokens.
4. For CI/CD, test the collector and archive behavior with the new adapter. For monitoring, prove the documented result limits and empty/stale/error handling. Add a regression case when changing semantics.
5. Document supported API versions, permissions, configuration, unsupported fields, and any external prerequisites. Distinguish fixture tests from real integration results.
6. Run `scripts/test.ps1` and `scripts/build.ps1`. Manually inspect a desktop demo after changes to the Wails bridge or window lifecycle.

Collection capabilities are `builds`, `queue`, `deployments`, and `metrics`. Additional names (`history`, `sync`, `health`, `instant`, `range`, `rules`) document the supported contract. Implement the complete selected interfaces; metadata cannot substitute for an implementation. Numeric build IDs and offset pagination remain a compatibility boundary, not a promise of universal CI support. See `internal/collector/extensibility_test.go` for a provider implemented outside the core package and composed without core changes.

Do not add provider-specific polling to the frontend, bake in cluster addresses, disable certificate validation, log credentials, or put real secrets into fixtures. Basic/bearer support must match the upstream protocol. Argo CD supports bearer tokens and explicit local-account session login, with one refresh after a session 401; it does not accept HTTP Basic as account login. OAuth/device login is not implemented.

## Where to change code

| Change | Location |
| --- | --- |
| Another platform using existing capabilities | `internal/providers/<name>/`, then `internal/providers/builtin.go` |
| Another query language using numeric series | Provider `Definition().Info.Query` and `MetricSource.Query` |
| A genuinely new data capability | `internal/platform`, `internal/collector`, and its frontend view |
| Archive behavior or backup | `internal/storage` |
| Chart display | `frontend/src/components/chart.ts` |
| Wails bridge and browser fixture | `frontend/src/bridge` |

The package layout and allowed dependency direction are documented in [ARCHITECTURE.md](ARCHITECTURE.md). `go test ./...` checks production import boundaries so a provider cannot silently take a dependency on storage or another adapter. Keep one root `go.mod`; these are source-level packages in the same application, not independently versioned plugins.

Use [Jenkins](internal/providers/jenkins/jenkins.go) for build/queue examples, [Argo CD](internal/providers/argocd/argocd.go) for deployments and session login, and [Prometheus](internal/providers/prometheus/prometheus.go) for numeric queries. If the new tool fits an existing capability and authentication flow, do not add provider-name conditions to the collector or frontend.

## Evidence for review

Include the problem, before/after behavior, a small reproducible fixture, tests run, and remaining limits. Source loss, duplicate observations, number reuse, and failed queries are more useful evidence than screenshots alone. The current results are recorded in `VERIFICATION.md`; update that record only for checks actually performed.
