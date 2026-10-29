# Architecture and extension boundaries

The application composes capabilities instead of inheriting a universal data class. A time series, a build execution, and a deployment snapshot have different identity and retention semantics. Flattening them into an untyped payload would move those differences into every caller.

## Repository layout

```text
dashboard/
├── main.go                      # Wails entry point and window setup
├── app.go                       # Composition root, Windows credentials, Wails dialogs/bridge
├── sdk/                         # Independent public contracts and optional HTTP helper
├── internal/
│   ├── application/             # Shared use cases, lifecycle, storage/credential ports
│   ├── platform/                # Core registry, persistence DTOs, SDK compatibility aliases
│   ├── collector/               # Polling policies; depends on capability and archive contracts
│   ├── storage/                 # SQLite settings, build archive, preferences, backup
│   ├── providers/
│   │   ├── builtin.go           # Explicit list of shipped provider definitions
│   │   ├── jenkins/             # API translation, metadata, fixture tests
│   │   ├── argocd/              # API translation, session authentication, fixture tests
│   │   ├── prometheus/          # API translation, PromQL presets, fixture/live tests
│   │   └── internal/httpapi/    # Core integration tests for SDK transport
│   ├── demo/                   # Synthetic HTTP services and integration smoke test
│   └── architecture/           # Production import-boundary regression test
├── frontend/src/
│   ├── main.ts                 # Navigation, page state, rendering and event coordination
│   ├── bridge/                 # Wails access, bridge contract, browser fixture
│   ├── components/chart.ts     # Numeric-series rendering and chart lifecycle
│   ├── components/dashboard.ts # Personal board editing, persistence, and card rendering
│   ├── components/dashboard-layout.ts # Versioned layout validation and collision placement
│   ├── components/dashboard-data.ts # Mounted-card query lifetime, refresh, and result state
│   ├── shared/                 # Data contracts and display formatting
│   └── style.css
├── frontend/tests/             # Browser acceptance tests
├── examples/                   # User configuration guidance
└── scripts/                    # Repeatable build and test commands
```

The application remains one Go module. `sdk/` is a separate, standard-library-only Go module, consumed through a local `replace` until a version is published. Each adapter owns its implementation, metadata and tests. Runtime plugin loading is not yet implemented.

## Dependency direction

Arrows mean “imports”; `app.go` is the composition root. The application package is independent of Wails, concrete storage, keyring, and built-in providers.

```mermaid
flowchart TD
    App["app.go / Wails bridge"] --> Application["application / use cases"]
    Application --> Collector["collector"]
    Application --> Platform["platform / contracts and registry"]
    App --> Storage["storage / SQLite"]
    App --> Providers["providers / built-in list"]
    App --> Platform["platform / contracts and registry"]
    Providers --> Adapters["jenkins / argocd / prometheus"]
    Adapters --> HTTP["sdk/httpapi"]
    Adapters --> SDK["public SDK contracts"]
    HTTP --> SDK
    Platform --> SDK
    Collector --> Platform
    Storage --> Platform
```

- `platform` depends only on the standard library and public SDK. It owns the core registry and persistence DTOs; existing data contracts are compatibility aliases. It cannot import providers, SQLite, or Wails.
- `application` imports only the standard library, the public SDK, `platform`, and `collector`. It owns the `Store` and `Credentials` ports; tests supply a temporary SQLite store and fake credentials. CLI/MCP adapters can call the same service without importing Wails. No CLI/MCP server is included.
- `collector` imports `platform` and the standard library. Its `BuildArchive` interface requires only `Known`, `SaveBuilds`, and `Running`; `application` passes its store port, and `app.go` supplies SQLite to that port.
- `storage` knows the shared data models, but not providers or collection scheduling.
- Each provider imports the independent SDK contracts and optionally `sdk/httpapi`. Production adapters cannot import any application package, including `internal/platform`. Integration tests may compose the core registry and adapters.
- `providers/builtin.go` owns the shipped registration list. A provider's `Definition()` owns metadata, construction, and optional provider-specific connection validation.
- The frontend's `bridge/index.ts` selects Wails or the browser fixture. Charts consume normalized numeric series, without querying upstream APIs.

`internal/architecture/boundaries_test.go` checks production Go imports during `go test ./...`. Integration tests may compose multiple layers. `TestProvidersBuildOutsideCore` also compiles the actual adapters in a separate consumer module with only an SDK dependency and network resolution disabled. The HTTP helper is public within the SDK; its production code has no core dependencies.

The extension model supports **compiled built-ins and trusted external ZIP packages**. The public SDK now contains the shared data interfaces and initial manifest/source-publication declarations. Compiled definitions now enter an atomic plugin registry, with persisted enable/disable and scoped source publication through explicit Source APIs. The host reads manifests for built-in and external providers; contributed UI is manifest-driven and external implementations use JSON-RPC stdio. See [SDK status](sdk/README.md).


## Data contracts

| Capability | Contract / normalized data | Identity and time | Storage |
| --- | --- | --- | --- |
| Build executions | `BuildSource` → `Build` | Connection + target + numeric build number + start time; milliseconds | SQLite archive; duplicate observations upsert |
| Queue | `QueueSource` → `QueueItem` | Current queue item and target; queued-at milliseconds | Latest snapshot in memory |
| Deployments | `DeploymentSource` → `Deployment` | Stable source ID, revision(s), independent sync/health and operation state | Latest snapshot in memory |
| Time series | `MetricSource` → `QueryResult` → labelled `Series` → `Point` | Label set + timestamp in Unix seconds; nullable numeric value | Queried from upstream; not a local metrics warehouse |

`Provider` supplies connection checking. `TargetDiscoverer` optionally supplies target discovery. `TestConnection` returns `{targets, discovery, warning}`: discovery is `complete`, `unsupported`, or `failed`. A failed probe is still an error; a successful probe with denied discovery retains that distinction. The editor preserves selected targets and accepts a directly entered target ID. Implement any combination of capabilities. For example, a build-only service does not need a fake queue method. A platform exposing both deployments and metrics implements both contracts; both are collected unless its descriptor sets `OnDemand: true`. An on-demand connection creates no background provider instance, request, or archive write, including on Refresh; explicit Query and TestConnection remain available. `CI`, `CD`, and `Monitoring` remain compatibility names for existing callers.

Targets may declare `capabilities`. A combined provider can return one target for `builds` and another for `deployments`, preventing deployment identifiers from being passed to build endpoints. An empty capability list retains the legacy behavior for single-purpose providers.

## Responsibilities

- `sdk/model.go` defines normalized DTOs and independent capabilities. Core `platform/registry.go` checks registration and interface/metadata consistency; `sdk/validation.go` checks shared connection fields.
- `collector/collector.go` orchestrates one connection. `collect_builds.go`, `collect_queue.go`, `collect_deployments.go`, and `collect_metrics.go` own their respective collection policies.
- `storage/sqlite.go` owns the unchanged archive schema, connection settings, preferences, and backup behavior.
- `providers/<name>` owns API paths, response decoding, platform authentication, supported configuration, and tests. PromQL presets live in `providers/prometheus/presets.go`.
- `application.Service` owns configuration changes, credential bindings through a port, admission, polling, manual operations, snapshots, history, preferences, and backup use cases.
- `app.go` owns service composition, OS credential access, the database path, demo startup, native dialogs, window geometry, and explicit Wails forwarding methods.
- `platform/persistence.go` contains storage-neutral record/filter/page DTOs. Storage aliases preserve existing Go callers; SQL, schema version, archive key, remain unchanged. Connection JSON now supports tenant and schemaVersion; unknown fields are preserved when editing.
- `components/dashboard-layout.ts` owns the strict `dashboard.layout` v2 preference format with v1 migration and backup: no overlapping cards, 12 columns, and at most 24 cards. A failed or future version is never rewritten by the frontend.
- `components/dashboard.ts` owns the dashboard editing transaction. It keeps a saved baseline plus undo history, commits only through the existing preference bridge, and restores the baseline on cancel. In a narrow board it presents cards vertically while retaining the wide-grid coordinates.
- `components/dashboard-data.ts` owns per-card data state. Build, queue, and deployment cards render already-collected snapshots. Metric, log, and trace cards issue explicit bounded queries only while their cards are mounted, serialize at most two active requests, discard stale layout results, and stop timers when the dashboard unmounts.

Provider identity does not select collection policy: implemented interfaces select capabilities. Protocol-specific validation uses the optional `Definition.Validate` callback; Argo CD local-account username validation lives with Argo CD. Authentication protocol implementation is still required when introducing a new authentication method.

## Failures and freshness

Each snapshot contains `modules[capability]` with `attempted`, `lastSuccess`, and `error`. A deployment failure does not mark a successful metrics or build response as failed. The aggregate error remains available for the connection banner, while rule freshness uses the metrics module. Published snapshots are deeply copied, including labels, sample values, warnings, and deployment revisions. Providers may reuse result memory on a later call, but must not mutate it concurrently with return/serialization. Callers cannot mutate the retained snapshot through GetState or Poll. Registry metadata and connection slices are also copied at ownership boundaries.

Capabilities run serially with a 12-second budget each inside a 60-second connection cycle. Queue time counts toward the cycle and manual-query deadline. Admission allows two operations globally and one per connection; a per-connection waiter does not hold a global slot. Cancellation remains cooperative. There is no distributed scheduler or background service.

Query text is interpreted by the adapter. The frontend uses declared language, default expression, and presets, and separates favorites by language. It renders normalized numeric series without parsing PromQL or SQL. The SQL fixture is an extension test, not an implemented SQL monitoring backend.

## Proven extension path

`collector/extensibility_test.go` is in the external `collector_test` package. It implements a combined provider using only public contracts, registers it through `Definition`, and collects builds, deployments, and synthetic SQL-shaped metrics without changing core collection code. The test checks target routing, absence of a queue, per-module failure isolation, immutable prior status, cancellation, and archive deduplication.

The browser suite independently injects a new provider name and SQL query metadata. It verifies the default expression, query execution, and the coexistence of target selection and metric-rule editing. These are contract proofs, not claims that a real fourth vendor has been integrated.

## Remaining boundaries

The first modularization deliberately preserves the existing SQLite format and historical collection algorithm. Build identifiers are still positive integers; pages are newest-first groups of 100 addressed by offset, with overlapping historical reads. A UUID-only build API or cursor-only pagination still needs a contract/storage change. Do not hash an opaque ID into a number or invent a start time to fit this contract.

A new build identity and provider-owned cursor remain design work. Decimalizing a numeric ID alone does not establish stable execution identity across attempts or reused numbers. Choose and validate a second CI's actual semantics before changing the archive key. No migration has been applied; an old executable is not a database downgrade strategy.

Logs and traces now have independent typed contracts and bounded on-demand explorers. Alerts, DORA calculations, and metrics retention remain unimplemented. Page rendering and event coordination remain in `frontend/src/main.ts`; the bridge, shared contracts/formatters, and chart component are separate modules. Existing capabilities can be extended without adding vendor-specific frontend polling. A new data capability still needs an explicit UI and collection contract.


## Accepted runtime decisions (2026-10-13)

This implementation adopts the application boundary, optional discovery, explicit on-demand policy, ownership copies, and cooperative lifecycle portions of the design brief. It does not mark all 22 proposed invariants as implemented.

- Keep typed observations and compiled-in definitions. Loki and Tempo are additional compiled-in adapters. The public SDK and restricted declarative schemas are implemented; trusted ZIP installation and subprocess runtime are implemented.
- Assign a fresh opaque runtime revision to each configured connection lifecycle. Edits, deletion, and credential replacement cancel the old lifetime. Failed persistence restarts the previous configuration under a fresh lifetime. Revision is visible in ConnectionView but is not a stored schema version.
- Polling owns one provider per connection lifetime; manual Query/TestConnection owns one provider per operation. Each closes an optional `io.Closer` after its last call. Built-in closers close idle HTTP connections. Failed registry capability validation also closes the constructed provider.
- Query and TestConnection register with the host before acquiring the two-slot gate. Their 8/45-second deadlines include queue time; provider creation and credential access happen after admission. Poll has a 60-second cycle budget including queue time and each module has 12 seconds. Load-based fairness and target-level fan-out budgets remain separate work.
- Close rejects new operations, cancels both polling and manual operations, waits for them, then closes storage. Cancellation is cooperative: in-process factories, credential calls, Close methods, and providers must finish; a provider that ignores cancellation can still delay shutdown.
- Cancelled manual operations discard both successful results and provider errors from the old lifetime. Poll publication checks the active runner. The UI also checks request/generation and connection state for late success and failure. Range chart cache has a 30-second TTL and 32-entry cap, includes connection revision/configuration, and checks validity before inserting results.
- TestConnection's structured result is a coordinated Go/TypeScript/preview wire change. Existing native consumer code must be rebuilt with this release. Archive wire shapes are retained; new log/trace methods use independent typed envelopes.

Regression evidence lives in `internal/application/service_test.go`, `internal/collector/ownership_test.go`, `frontend/tests/provider-lifecycle.spec.ts`, and the application import-boundary check. Tests use synthetic providers/local data, not live vendor acceptance or an independent SDK.

Still open: target-level partial outcomes and migration of existing sources to common envelopes; build identity/cursor semantics and schema migration; orphan credential cleanup after OS-store failures; nested/advanced configuration schemas; and full secret redaction of arbitrary third-party diagnostics. The current numeric build/offset and Argo-shaped deployment limitations remain explicit.


## Log and trace observations (2026-10-13)

`LogSource` and `TraceSource` are independent of `MetricSource`. Each ships with a separate adapter package, static descriptor, HTTP fixtures, and an on-demand UI. `application/observations.go` stamps connection/revision/request/window/observation time and returns typed outcome/completeness/diagnostics alongside rows. Search windows use epoch seconds; event ns, trace/span IDs, and OTLP int64 values stay strings. Returned maps, slices and raw attribute bytes are copied.

Loki retains labels, structured metadata, duplicate lines and equal timestamps. Search is capped at 500 rows and 24 hours. It does not implement tail or a synthetic timestamp cursor. Tempo searches at most 100 summaries and reads at most 2000 spans per detail response. Detail lookup omits search-window filters, accepts both Tempo `batches` and OTLP `resourceSpans`, normalizes base64/hex IDs, and preserves original span/scope fields. Neither sampling completeness nor missing-parent root status is inferred. Shared transport still limits responses to 8 MiB.

Connection JSON schema 0 (legacy) and 1 are accepted; saves write 1. Storage edits preserve unknown top-level fields and target/rule fields matched by ID, without exposing unknown settings to Desktop. Future connection schemas are readable but cannot be opened or overwritten. SQLite remains schema v1; there is no archive migration. Tenant binding uses the same new-connection boundary as endpoint changes. See [feature status](FEATURE_SPEC_AND_STATUS.ko.md) for the full invariant and ADR mapping.

## Personal dashboard layout (2026-10-15)

`dashboard.layout` is a UI preference, separate from connection configuration, credentials, and archived builds. Its version 2 JSON contains ordered card definitions, grid coordinates and extension bindings. Valid v1 layouts migrate without moving cards and are backed up as `dashboard.layout.v1.backup` before a v2 save. The native SQLite preference API stores it unchanged as text; no backend table, Go model, migration, or new dependency is required. An empty layout is a meaningful saved preference. If parsing or validation fails, the UI keeps the stored text intact and disables editing rather than attempting a speculative replacement.

The board is 12 columns wide with at most 24 non-overlapping cards. `dashboard-layout.ts` performs validation and placement. `dashboard.ts` handles pointer and keyboard editing, settings, save/cancel/undo state, responsive stacking, and card DOM. `dashboard-data.ts` separates snapshot cards from query cards: snapshots reuse collector state, while metrics, logs, and traces run dashboard-owned reads on opening, manual refresh, or an optional mounted-card interval. The latter is bounded to two concurrent requests and invalidates results when a card or its source configuration changes.

The `json` node originally extended layout v1 and remains supported in v2 with an exact `json` configuration (`source: http`, URL, jq filter), requires manual refresh and preserves existing node formats. Older clients reject this new kind rather than rewriting it. `api-query.ts` captures configuration, executes a manual preview and seeds the added node with that result without issuing a duplicate request. Results are not persisted. The UI invalidates old replies when settings or pages change.

`App.ReadJSON` retains its original request/result contract. `Service.ReadJSON` reuses admission, cancellation and shutdown tracking with a 25-second overall budget, resolves the enabled JSON plugin, and leases a transient SDK `JSONSource`. `internal/providers/jsonapi` owns unauthenticated GET and embedded gojq evaluation. Saved JSON connections additionally expose table/scalar `DataSource` contributions. Legacy API cards require no saved connection or credential store; no shell process is started. HTTP is bounded to 20 seconds/8 MiB, and jq receives a cooperative 2-second context with 200 outputs/1 MiB display bounds. This is an in-process interpreter, not a resource-isolated subprocess sandbox. JSON scalar values cross the frontend bridge as exact text, not JS numbers.

## Plugin and source lifecycle (phase 2)

`platform.Registry` protects definition, ownership and activation metadata with a
read/write lock. Compiled definitions are wrapped in manifests using the existing
provider kind as plugin ID. Registering a multi-provider plugin validates all
entries before publishing any. Metadata is deeply copied; factories run outside
the registry lock. Known legacy feature labels (`history`, `sync`, `health`,
`instant`, `range`, `rules`) remain available to the UI but are not source
interfaces.

Application coordinates activation through `SetPluginEnabled`. It saves the
`plugins.disabled` preference before stopping affected runners; a failed save
leaves working instances intact. Disabling retains connection settings,
credentials, dashboard preferences and archives, cancels queued/active operations
and leaves an unavailable snapshot. Startup restores disabled state before
starting collection. Package installation, removal and replacement take effect after restart; see
[external package lifecycle](EXTERNAL_PLUGINS.md).

Each connection runner can lazily own a `platform.Sources` scope. The provider
receives only the SDK registrar. Register/update validates exact source
capabilities and stable target identity; updates cancel the prior generation
synchronously. Leases retain implementations until their last call ends. Sharing
one pointer across sources closes it only after its final registration and lease.
Rejected registrations remain caller-owned. Retired implementations leave the
resource map so repeated updates do not retain closed clients.

`Sources` and `QuerySource` are explicit Wails APIs; metric queries use existing
connection/global admission. Publication failure rolls back all accepted sources
and can be retried. These APIs are the first consumers of the new source registry.
All six built-in providers publish `default`. Background collection and legacy
metric/log/trace queries lease their declared CollectionSource. Source changes
wake collection and reset transient state; cancelled build responses are checked
before archive writes. Injected definitions without CollectionSource retain the
legacy compatibility path. Failed external processes are recreated on the next call.

## Declarative contributions (phase 4)

Manifest registration validates IDs, references, renderer/capability compatibility
and schema keywords atomically. The frontend renders plugin management, dedicated
dashboards and query forms. Global dashboard v2 cards retain their bindings when
unavailable. ExecuteContribution resolves a live declaration, validates/defaults
input, acquires a source under connection admission, dispatches a typed capability
and copies a bounded result. It rejects stale source, connection and plugin
activation revisions, including a UI-only plugin reading another provider's source.
Disabling a UI-only plugin cancels its active calls and rejects stale results
without closing another plugin's provider.

SDK DataSource provides table/scalar/timeseries for new domains. Existing typed
sources keep their contracts and archive semantics; core converts their records
for declarative display. Schemas are closed flat objects, shared by forms and
backend validation. Non-secret provider values live in Connection.Config and are
copied with settings. See [SDK contract](sdk/README.md) for limits and keywords.

Trusted ZIP loading and JSON-RPC stdio execution are implemented. Jenkins,
Argo CD, Prometheus, Loki, Tempo and HTTP JSON all own their manifests and source
publication. Core consumes SDK interfaces; providers import no core implementation.
Arbitrary frontend scripts and upstream writes are unsupported. External processes
provide a fault/lifecycle boundary, not an OS sandbox.
