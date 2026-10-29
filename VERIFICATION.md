## Full provider migration, phase 6 — 2026-10-29

- `bash scripts/test-go.sh`: SDK/root tests and vet passed. All six adapters
  compile in an independent consumer without core imports. Local HTTP integration
  exercises publication, legacy queries and declarative contributions together.
- HTTP JSON/jq now belongs to its provider. Tests retain exact large integers,
  scalar/table validation, admission and cancellation on JSON plugin disable.
- `npm run build` passed. Full browser run: 76 passed and 2 onboarding failures;
  moving JSON after the existing preview default fixed both, and `--last-failed`
  passed 2/2. No other frontend changes followed that correction.
- Wails v2.11.0 Windows/amd64 production build succeeded:
  `build/bin/idp-dashboard-plugins.exe`. The independent external example ZIP is
  available at `build/bin/plugin-example/example-inventory.zip`.
- Native installation dialogs and authenticated live services were not exercised
  in this phase. Race checking remains unavailable with the installed 32-bit GCC.
  External native code is trusted code, not OS-sandboxed code.

Earlier sections below record the state at their respective verification dates.

## External package/runtime phase 5 — 2026-10-29

Implemented trusted ZIP staging, atomic next-start index, startup registration,
restart-applied install/upgrade/removal, native file picker and package status UI.
SDK-only JSON-RPC stdio supports all existing typed capabilities, reverse scoped
source changes, cancellation, process failure recovery, partial errors and secret
redaction. Package removal retains user settings/history and immutable package files.

SDK/root Go tests and vet passed. Independent fixture executables were built with
GOPROXY=off and exercised as real processes: capability mismatch, source update/delete,
crash/recovery through application, cancellation of an uncooperative call, activation
and secret redaction. ZIP validation and restart boundaries passed. Frontend build
and 8 focused browser tests passed. The working example ZIP was built independently.
Native picker/WebView2 interaction and live upstreams were not exercised. Existing
32-bit GCC still prevents race builds. This is trusted code, not an OS sandbox.

## Declarative plugin contributions — 2026-10-28

Phase 4 adds manifest-derived navigation, plugin management, dedicated dashboard
and query forms, global extension cards, typed table/scalar/timeseries rendering,
and non-secret provider config forms. Layout v1 migrates to v2 with original
backup and retains unavailable plugin bindings. The SDK and providers stay
independent of core imports; actual providers still compile outside the core.

Validation:
- Full SDK/root Go tests and vet passed (`bash scripts/test-go.sh`). Additional
  application/platform checks passed after adding cross-plugin stale-response
  coverage. Custom SDK-only DataSource exercises defaults, invalid-input rejection,
  enable/disable, source deletion and activation-revision invalidation.
- `npm run build` passed; the existing bundle-size warning remains (~657 kB).
- Full Playwright suite: 74 passed in both wide/compact viewports. New cases cover
  generic renderers, escaped HTML/large opaque IDs, schema validation, v1 backup,
  saved card bindings and disable/re-enable recovery. Initial new-form test used
  the wrong button label; its locator was corrected before the passing full run.
  After final spacing/gap/boolean refinements, all 6 contribution tests passed;
  wide/compact screenshots were inspected. Final application tests also passed,
  including real Jenkins contribution dispatch and int64-to-text preservation.
- Native WebView2 and live services were not exercised; no release executable was
  rebuilt. Race tests remain unavailable with the installed 32-bit-only GCC.

External ZIP installation/stdio execution and remaining provider migrations are
phases 5 and 6. No claims of hot package installation or process isolation yet.

## Jenkins source migration — 2026-10-28

Jenkins now declares its plugin manifest and publishes its default source. The
collector leases that registration, rebuilds its cursor state on source revision
changes, and wakes on source updates/deletion. Cancelled remote build responses
are discarded before archive writes. Other providers retain their legacy path.
`bash scripts/test-go.sh` passed, including Jenkins HTTP fixture collection,
publisher invocation, disable/re-enable identity/archive preservation and late
cancelled-build rejection. Native WebView2/live Jenkins were not exercised.

# Verification record

## Plugin activation and scoped source lifecycle — 2026-10-28

- Added atomic plugin registration, definition ownership, copied metadata and
  synchronized activation state. Existing compiled providers enter the same
  registry using their current kind as plugin ID. The application persists
  `plugins.disabled` before stopping runners and preserves connections, credentials,
  preferences and archives. Failed setting writes leave active runners intact.
- Implemented the SDK SourceRegistrar in a per-connection core scope. Tests cover
  invalid updates preserving live sources, synchronous generation cancellation,
  shared-pointer lifetime, source ownership, target-ID reuse rejection, delayed
  result suppression, publication rollback/retry and access revocation on disable.
  Known legacy descriptive tags remain distinct from actual source capabilities.
- Wails, TypeScript and the preview fixture expose Plugins, SetPluginEnabled,
  Sources and QuerySource. Source publication is lazy; QuerySource currently
  supports metrics. Existing collector and legacy query calls remain on their
  compatibility path until individual provider migration. No plugin management
  screen, external executable loader or contributed UI renderer is included yet.
- `bash scripts/test-go.sh` passed: SDK tests/vet, all application Go tests/vet,
  import boundaries and independent builds of all five provider adapters.
- `npm run build` passed (existing bundle-size warning). The new lifecycle test
  passed at both viewport sizes. Full Playwright run: 66 passed, 2 failed before
  functional assertions because the user manually closed Edge during newPage/goto (confirmed afterward). Both failed
  cases passed in a fresh `npm test -- --last-failed` run; no production fix or
  retry configuration was used to mask the browser failures.
- Race tests were attempted but could not compile: the installed GCC reports
  `64-bit mode not compiled in`. Native WebView2 and live infrastructure were not
  exercised, and the release executable was not rebuilt.

## Public extension contracts — 2026-10-28

- Extracted existing DTOs, capability interfaces, validation and ownership
  helpers to the independent `sdk/` Go module. Core aliases preserve type identity
  and serialized fields. Moved the provider HTTP client to `sdk/httpapi` without
  changing transport behavior. Persistence and runtime registry stay in core.
- All five provider production packages now import the SDK instead of core
  internals. The architecture test compiles their actual sources in an independent
  consumer module with only a local SDK dependency, `GOWORK=off`, `GOPROXY=off`
  and `GOSUMDB=off`; this passed.
- Application `go test ./... -count=1` and `go vet ./...` passed. SDK
  `go test ./... -count=1` and `go vet ./...` passed separately. Tests include the
  existing collector, SQLite, provider and HTTP fixtures plus independent SDK
  client and import-boundary checks. `git diff --check` passed.
- Added manifest, declarative UI contribution and scoped source-publication
  contracts. These are unreleased declarations only: dynamic registry lifecycle,
  manifest loading, source publication hooks and external execution are not yet
  wired into the app. The runtime continues to use built-in definitions.
- No frontend, schema or credential change. Browser tests, native WebView2,
  live infrastructure and race tests were not run in this backend extraction.
  No release executable was rebuilt. The SDK has not been published.

## Time-axis labels and HTTP jq nodes - 2026-10-15 16:29 KST

- Ctrl+wheel now changes only the time range. Vertical bounds and node dimensions stay fixed. The separate “최근 값” heading was removed; the last visible sample value sits beside the graph on the right at its plotted height. The final correction uses the same 11px regular text as the axis labels. Three vertical ticks and nearest-sample time/value hover inspection are implemented.
- All 66 Playwright cases passed at 1440x1000 and 480x760. After the final font-only change, both targeted graph tests passed again, including matching font size, right-side placement, hover, fixed vertical scale, node geometry and no extra query. HTTP UI tests cover scalar rendering, escaped text, saved nodes, manual-only execution, source restrictions and stale response suppression. The final browser preview was visually inspected.
- `go test ./... -count=1` and `go vet ./...` passed. Synthetic HTTP tests cover jq paths/sums/streams, large integer preservation, invalid JSON, rejected credentials/commands, redirects, input/output limits and cancellation. The application test verifies shared admission and cancellation during shutdown. No real API or CLI credentials were used.
- TypeScript and the production frontend build passed (643.10 kB / 217.88 kB gzip; existing non-fatal bundle warning). Wails 2.11.0 rebuilt `build/bin/idp-dashboard.exe` at 16:29:52 KST, 16,799,232 bytes. The binary contains `index-C03cOVJD.js`, `index-Bx5Ft7Zp.css` and the `ReadJSON` bridge.
- Command execution and API authentication are not implemented, following the user's explicit choice to ship unauthenticated HTTP first. The API tab exposes Command as disabled. YAML is not supported. Native WebView2 interaction and live external API acceptance were not exercised in this pass; browser tests use a bridge fixture, backend tests use loopback HTTP. Race tests were not repeated. gojq cancellation is cooperative, not process isolation.

The final test run used a fresh Vite process. An earlier run had four failures because test imports and Vite's hot-reloaded bridge held different instances; restarting the dev server resolved them without changing production code. The final binary includes the latest font correction and supersedes the builds below.

## Graph zoom build - 2026-10-15 15:57 KST

The metric graph supports node-local Ctrl+wheel zoom (100–800%), zoom buttons, and reset, with stronger line/grid contrast. Two targeted Playwright cases passed at wide and compact sizes: graph zoom changes neither node geometry nor other graphs, issues no extra query, respects bounds, and preserves control focus. The production frontend build passed (635.61 kB / 215.52 kB gzip). Wails rebuilt `build/bin/idp-dashboard.exe` at 15:57:50; the file is 15,886,336 bytes and contains the latest `index-DADZ-TzP.js` asset. The already-running native process must be closed and relaunched to load this build. Native interaction has not been checked. JSON/jq source nodes are not implemented in this build.

## Personal dashboard layout - 2026-10-15

- TypeScript checking and the Vite production build passed. The final bundle is 633.55 kB (214.65 kB gzip); the existing 500 kB bundle warning remains non-fatal. No dependency was added.
- All 58 Playwright cases passed in the final run, across 1440x1000 and 480x760. Coverage includes card configuration and duplicate metric queries, pointer/keyboard layout editing, reload restoration, intentionally empty layouts, failed save/retry, future-schema preservation, focused editing during polling, missing connections, long target identities, collision placement, query bounds and escaped log text. Data tests also cover refresh timers surviving state updates, two-request concurrency, stale replies, stop cleanup, and nullable/small metric values. Existing explorer, archive, connection and authentication regressions passed.
- `go test ./... -count=1` and `go vet ./...` passed with a writable temporary Go cache. Backend source and SQLite schema were unchanged.
- The Windows amd64 executable was rebuilt from the final frontend using the pinned Wails 2.11.0 command: `go run github.com/wailsapp/wails/v2/cmd/wails@v2.11.0 build -platform windows/amd64 -skipbindings -s`. Output: `build/bin/idp-dashboard.exe`.
- The production browser preview was visually checked at wide and narrow sizes. The default metric cards show graph bounds, time range, and series labels without internal clipping. Narrow layout has no page-width overflow.
- `git diff --check` passed. README, architecture, and F21 describe the editing and persistence contracts.

The browser tests used fixtures and a simulated preference bridge. This change has not been manually exercised in native WebView2 or against live infrastructure; existing SQLite preference behavior is reused. Browser preview preferences remain in memory and are separate from native persistence. No production database or credential was used. Race tests, release packaging/signing, and long-running load checks were not repeated for this frontend-only change. Sandbox restrictions required temporary Vite/test-output directories and a separate Playwright configuration with the same test files, Edge channel, and viewport sizes; the standard test script was not run end to end in this session.

## Loki/Tempo and core contracts - 2026-10-13

Implementation commits: `6514950`, `ac9ecd6`, `72c9ba3`, followed by the on-demand freshness display correction. Feature-level requirements and remaining scope are recorded in [FEATURE_SPEC_AND_STATUS.ko.md](FEATURE_SPEC_AND_STATUS.ko.md).

- `scripts/test.ps1` passed: dependency install, TypeScript/Vite build, 28 Playwright cases at 1440x1000 and 480x760, Go tests, and `go vet ./...`. After the final on-demand display correction, the frontend production build and all 28 browser cases passed again.
- `go test -race ./... -count=1` passed with the final backend. HTTP fixtures cover Loki tenant/auth headers, precise ns and duplicate timestamps, metadata, query rejection; Tempo search/detail, both batches/resourceSpans shapes, base64/hex IDs, typed int64/resource/span fields and unsafe trace-ID rejection. Application tests cover partial envelopes, cancellation, tenant credential binding, per-connection admission, and shutdown.
- Storage fixtures preserve unknown top-level/target/rule fields, remove deleted entries, clear optional fields, and reject future connection schemas without changing the stored JSON. Collector fixtures verify that failed, empty, future-dated and incomplete metric results do not update last success or imply health.
- The native `--demo` now exercises five real adapters over synthetic loopback HTTP. A Windows smoke run used a new LOCALAPPDATA under ignored `build/bin/smoke-observations-*`: the app showed five connections, rendered a Loki row with an exact 19-digit ns timestamp, executed Tempo search, and displayed trace detail with a 12000000 ns span. It exited normally after Alt+F4; the pre-existing user app remained running. This smoke exposed the on-demand freshness display issue, subsequently fixed and covered by the browser regression.
- `scripts/build.ps1` produced the Windows amd64 executable. The Vite bundle remains approximately 599 kB (204 kB gzip), with the existing non-fatal size warning.

No live Loki/Tempo server, real keyring entry, production database, release compatibility matrix, or large-connection load test was used. CLI/MCP and subprocess plugins are deferred. SQLite archive schema and identity remain v1; the new connection JSON schema marker is a separate version axis. The native smoke preceded the final on-demand-only display correction; the rebuilt artifact includes that tested correction.

## Provider architecture rebuild - 2026-10-13

Baseline: `77c9868dbd88185203da87ca4cee7235fccde3f2`, initially clean tracked worktree. This section records the application/lifecycle/ownership/discovery checkpoint committed as `9aadaf1`; the later Loki/Tempo work is recorded above. Environment: Windows amd64, Go 1.25.7, Node 24.13.0, Wails CLI 2.11.0.

| Check | Actual result |
| --- | --- |
| Baseline | `go test ./... -count=1` passed before changes. |
| Repository checks | `scripts/test.ps1` passed: npm ci, TypeScript/Vite production build, Playwright 20/20 (wide and compact), Go tests, go vet. |
| Final race/static checks | `go test -race ./... -count=1` and `go vet ./...` passed on the final source, including the additional registry ownership/cleanup test. |
| Lifecycle and ownership | New fixtures check on-demand idle behavior, absent/denied discovery with direct reads, edit/delete/credential replacement cancelling late success, provider Close, shutdown of active/queued operations, admission deadlines, immutable nested snapshots, and registry metadata ownership/rejected-instance cleanup. |
| Browser regressions | Late query success and failure cannot overwrite the selected connection. Discovery denial preserves selected targets and permits direct target entry/save. Existing 14 browser cases also pass. |
| Windows executable | `scripts/build.ps1` passed; `build/bin/idp-dashboard.exe`, 15,746,560 bytes. The first attempt hit transient npm EBUSY on a node_modules file; retry completed without a source change. |
| Native demo | Rebuilt executable ran with --demo and a separate LOCALAPPDATA under build/bin/smoke-data-*. Wails/WebView2 displayed three connections, collected builds/deployments, CPU 42 and memory 67, and range charts through real Go adapters against synthetic loopback APIs. The test process exited after a normal window close. Native connection-editor interaction was not completed. |
| Diff hygiene | `git diff --check` passed. |

The frontend bundle remains above Vite's 500 kB warning threshold (590.43 kB / 201.29 kB gzip); this was not treated as a build failure. An early race run overlapped frontend asset replacement and failed the root embed step; subsequent runs use stable built assets.

IDP_TEST_KEYRING and IDP_TEST_PROMETHEUS_URL were unset for test execution. OS keyring acceptance and live infrastructure checks were skipped. Existing user database/credentials were not opened by these tests; native verification used its own temporary demo store. No schema/archive migration, external plugin runtime, new vendor, or real-infrastructure acceptance is claimed. Historical evidence below is retained separately and is not evidence for this rebuild.

## Historical record - 2026-10-03

Recorded on 2026-10-03. Results below describe this working tree on one Windows x64 PC. Fixtures, native smoke checks, and live integration checks are intentionally distinguished.

## Performed

| Check | Evidence / result |
| --- | --- |
| Go tests | `go test ./... -v`: adapters, archive, collector, demo API integration, and optional credential lifecycle passed; live Prometheus is skipped unless opted in |
| Static checks | `go vet ./...` passed |
| Frontend build | TypeScript checking and Vite production bundle passed |
| Browser UI | Playwright at 1440×1000 and 480×760: onboarding, discovery/selection, history filter/retention, query/favorite/error, independent Sync/Health, responsive width, focus-preserving live updates, and failed metric summary |
| Windows executable | Wails 2.11.0 production build for Windows amd64 succeeded |
| Native demo smoke | Earlier build opened in WebView2 and rendered all three adapters through Go; real demo values (CPU 42, memory 67), connection state, and source timestamps were visible. Final small UI changes were covered by browser tests and rebuilt; another native inspection was stopped by the user's Escape input |
| Windows credential store | Opt-in test created a random test token, read it back, used it for a demo connection check, verified the SQLite export did not contain it, and deleted its credential entry |
| Real Prometheus | Read-only HTTP query against the reference environment's Prometheus 3.4.1: `up` over one hour returned 22 series and 241 samples in the first series |
| Real connectivity checks | Jenkins returned 403 without credentials; Argo CD returned 401 without credentials. This establishes reachability, not authenticated API compatibility |
| npm dependencies | `npm audit` reported zero vulnerabilities after updating the initial Vite/ECharts pins; this is not a full security audit |

The native demo server and browser preview are synthetic. No real Jenkins builds or Argo CD applications were modified. Real Jenkins/Argo CD authenticated collection is **not yet verified**.

## Covered failure semantics

- Jenkins nested folders, encoded branch names, Basic auth, pagination bounds, and optional commit metadata.
- TLS fails for an untrusted certificate and succeeds when the explicit test CA is provided.
- Redirects are not followed; authentication secrets are not echoed in HTTP error messages.
- Prometheus non-finite numeric samples become gaps, and excess series are truncated with a warning.
- Argo CD `OutOfSync` and `Healthy` remain independent.
- Duplicate build observations, reused numbers with different start times, and separate connections do not overwrite distinct archive entries.
- Delayed running observations do not regress completed builds. Missing running builds become `UNCONFIRMED`; source failure retains archive data and freshness evidence.
- SQLite export restores the expected records and refuses to overwrite an existing file. A newer database schema is rejected without rewriting its version.
- Demo HTTP integration imports 320 build summaries through the actual adapters/collector.

## Live acceptance still needed

1. Add a read-only Jenkins account in the app. Verify actual organization folders and multibranch jobs, queue visibility, a running-to-completed transition, and backfill against the source UI.
2. Add an Argo CD read token. Compare application selection, namespace/name identity, Sync, Health, and revision with the source UI. Check permission-restricted projects explicitly.
3. Disconnect the PC route temporarily using a user-controlled test environment, then restore it. Confirm freshness/error recovery and retained local records.
4. Minimize the app, observe collection progress after restoring it, then test actual Windows sleep/resume. These lifecycle cases have implementation hooks but have not been accepted on a real sleep cycle.
5. Verify backup/restore through the Windows file dialog and across another Windows account, re-entering credentials there.
6. For a public release, test a clean Windows installation, choose a license, replace default packaging artwork/metadata if desired, and decide code-signing/distribution policy. The current executable is a development artifact, not a signed release.

## Package architecture refactor (2026-10-03)

- Backend source and adjacent fixture tests now live in `platform`, `collector`, `storage`, and individual `providers/<name>` packages. The app and demo use the new import paths.
- `go test ./... -count=1` and `go vet ./...` passed after the final package changes. This includes import-boundary checks, registry/provider validation, session login/refresh, missing-build retention, unchanged SQLite archive/backup behavior, and the combined-provider extension fixture.
- The shared HTTP test verifies both rejection of an untrusted certificate and success with an explicitly provided test CA. Tests use disposable local fixtures; no production credentials are required.
- Frontend bridge/contracts, shared display helpers, and chart rendering were extracted into folders. TypeScript/Vite production build and all 14 Playwright cases passed across both viewport sizes.
- Wails 2.11.0 built the Windows amd64 executable using the tested frontend assets. A new native UI smoke session and live authenticated upstream checks were not run for this refactor.
- No schema migration or dependency update was introduced. Numeric CI IDs, offset paging, source-compiled adapters, and page coordination in `frontend/src/main.ts` remain explicit boundaries.

## Reproduce

### Collection modularization (2026-10-03)

- Go tests and `go vet ./...` passed after the refactor. Existing Jenkins backfill, archive retention, Argo authentication, and Prometheus fixture tests remain passing.
- An external-package fixture registers a new combined provider and supplies build, deployment, and SQL-shaped metric data without core collector edits. It verifies optional queue support, capability-specific target routing, isolated failures, previous snapshot immutability, cancellation, and archive deduplication.
- Registry tests reject duplicate provider kinds, inaccurate capability declarations, and metrics without query metadata.
- All 14 browser cases passed across both window sizes. The added case uses a new platform kind and SQL metadata, executes a query, and confirms that target selection and metric rules coexist in a combined provider.
- This proves the extension contract with fixtures; it does not add or validate a real fourth vendor. No database migration was performed. Numeric build IDs and offset pages remain constraints.

### Argo CD authentication correction (2026-10-03)

The connection form previously offered HTTP Basic for Argo CD, which does not perform Argo CD account login. Supported methods are now provider-specific. An explicit local-account login exchanges username/password at `/api/v1/session`, caches the returned bearer token in memory, and refreshes once after a 401. A rejected password is not automatically retried on subsequent polls. Changing authentication method requires a new credential entry.

Go HTTP fixture tests passed for login payload and proxy prefixes, session reuse/expiry, rejected-password retry suppression and error redaction, existing bearer operation, permission errors, and Basic rejection. UI tests passed in both window sizes, including choosing Argo CD login versus bearer and clearing the secret on authentication changes (12 UI cases total). Real credentials from the screenshot were not read or used; authenticated production acceptance remains pending.

Run `scripts/test.ps1`, then `scripts/build.ps1`. See the optional integration commands in `README.md`. Browser screenshots and failed-test traces are written under `frontend/test-results`; these generated files are ignored by Git.

The Vite build currently emits a non-fatal bundle-size warning (approximately 585 kB of JavaScript before gzip). No network download is needed for those assets at runtime because they are embedded. Performance under many simultaneous real connections has not been measured.
