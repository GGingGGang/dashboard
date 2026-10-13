# Verification record

## Provider architecture rebuild - 2026-10-13

Baseline: `77c9868dbd88185203da87ca4cee7235fccde3f2`, initially clean tracked worktree. This record covers the uncommitted application/lifecycle/ownership/discovery changes described in ARCHITECTURE.md. Environment: Windows amd64, Go 1.25.7, Node 24.13.0, Wails CLI 2.11.0.

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
