# IDP Dashboard

A Windows desktop dashboard that a developer can keep open while working. Arrange build history, deployment status, metrics, logs, and traces into a personal operations view, then connect to existing tools by URL and read-only credentials. No dashboard server, Kubernetes controller, or OCI account is required.

This is an early implementation of an operations view for a developer platform, not a complete internal developer platform. The interface is Korean; contributor documentation is English.

## Run

Requirements: Windows x64 and Microsoft Edge WebView2 Runtime. The executable embeds the frontend.

```powershell
.\build\bin\idp-dashboard.exe
# Separate demo database and synthetic loopback APIs:
.\build\bin\idp-dashboard.exe --demo
```

In **연결 설정 → 연결 추가**, select a provider, enter its base URL and authentication, then choose **연결 검사 · 대상 찾기**. Select jobs or applications to watch and save. Prometheus connections instead have optional query rules. Multiple connections of each provider are supported. If automatic discovery is unavailable or denied, the editor preserves selected targets and allows direct entry of a known target ID; target access is still checked by the upstream API.

Demo mode exercises the same Go HTTP adapters and SQLite collector as real connections. Its sample data is explicitly labelled. It creates a separate database and never connects to your infrastructure unless you manually add a real connection. Demo connections are reset on each launch.

## Current integrations

| Provider | Read operations | Configuration |
| --- | --- | --- |
| Jenkins | Folder/multibranch discovery, build summaries, queue, historical backfill | Controller base URL, typically Basic auth with username/API token |
| Argo CD | Application discovery, Sync, Health, revision, last operation | Server base URL and bearer token, or explicit local-account session login |
| Prometheus | Instant and range queries, threshold rules | Prometheus base URL; none, Basic, or bearer authentication |
| Loki | Bounded LogQL log search | Loki query base URL; optional tenant ID; none, Basic, or bearer authentication |
| Tempo | TraceQL search and trace-by-ID detail | Tempo query base URL; optional tenant ID; none, Basic, or bearer authentication |

For Loki and Tempo, save the connection and open **로그 탐색** or **트레이스 탐색**. These connections run on demand. Queries cover up to 24 hours, with up to 500 log rows or 100 trace summaries; trace detail displays up to 2000 spans. A reached limit does not imply an exact total or a lossless next page. Trace detail may be sampled or incomplete. Nanosecond timestamps and trace/span IDs remain strings. Log lines and structured fields are displayed as text. Logs and traces are not archived locally.

The optional tenant ID sets `X-Scope-OrgID` for the configured connection. Changing a saved endpoint, provider, or tenant requires a new connection. Upstream authorization still determines accessible data. No Loki tail, LogQL metrics, TraceQL metrics, or automatic cross-provider correlation is implemented.

The implemented HTTP shapes follow the [Loki HTTP API](https://grafana.com/docs/loki/latest/reference/loki-http-api/) and [Tempo HTTP API](https://grafana.com/docs/tempo/latest/api_docs/). Local HTTP fixtures cover these shapes; no live Loki/Tempo release has been certified. See [기능 명세와 구현 현황](FEATURE_SPEC_AND_STATUS.ko.md) for the design-brief mapping and remaining work.

Resource requests are read-only. Explicit Argo CD account login uses `POST /api/v1/session` to obtain a session token; the app cannot trigger builds, sync applications, or change monitoring configuration. Current integrations use existing HTTP APIs and need no upstream plugin. Authentication and network access still have to be configured on each upstream service.

For Argo CD, choose **Bearer · API 토큰** for an existing token, or **Argo CD 로그인 · 사용자명 + 비밀번호** for a local account such as `admin`. HTTP Basic is not Argo CD account login. The login option stores the password in Windows Credential Manager, keeps the issued token in memory, and retries an expired session once. Rejected login credentials stop automatic login attempts until the connection is saved again. SSO is not supported. Existing Basic connections require selecting a supported method and re-entering the credential; credentials are not silently reinterpreted. See the [Argo CD API documentation](https://argo-cd.readthedocs.io/en/stable/developer-guide/api-docs/).

Supply a **base URL**, including a reverse-proxy prefix if applicable; do not append `/api/json` or `/api/v1`. Jenkins requires its controller root, not a job/folder URL. An optional browser URL allows source links to use another address. An optional PEM CA file extends the system trust store. TLS verification is always enabled; redirects are rejected to avoid forwarding credentials to another endpoint. Configure the final URL directly.

Connections are made from the PC. A private Kubernetes ClusterIP requires an existing route, VPN, or explicitly managed port-forward. The app does not create these. See [configuration examples](examples/README.md).

## Views and interpretation

- **내 대시보드**: a personal, persistent board of build, queue, deployment, metric, log, and trace cards. Use **배치 편집** to add a card, change its source connection and optional target or query, then give it a display name. On a wide window, drag a card title to move it, drag its lower-right corner to resize it, or focus the move/resize control and use arrow keys. Card settings also set width and height. **되돌리기** reverses the latest layout edit; **취소** restores the last saved layout; **저장 · 배치 완료** writes the whole layout. Narrow windows stack cards vertically: change card size in settings and edit the wide-screen position after widening the window.
- **운영 현황**: latest failed completed build per job, selected queue items, independent deployment Sync/Health, metric rules, and source freshness. Service/environment labels map selected CI/CD targets; metric queries retain their own label scope.
- **빌드 이력**: local build summaries, filters, pagination, and consistent SQLite backup. Console logs and artifacts are not archived.
- **지표 탐색**: PromQL instant/range queries, charts, values, and query favorites.
- **API 쿼리**: manually fetch unauthenticated HTTP GET JSON, apply a jq expression, then add the result to the dashboard. For `{"service":{"file":{"name":123123}}}`, use `.service.file.name` to display `123123`. Use `.` for the full response or `[.items[].cost] | add` for a sum. Command and API authentication are deferred; the disabled Command option is not an executable source.
- **연결 설정**: multiple instances, credentials, target discovery, service/environment mapping, and editable rules.

A dashboard can contain up to 24 cards on a 12-column wide-screen grid. Each card stores its own source connection; snapshot cards may narrow to a selected target. Metric, log, and trace cards also store a query, 15-minute to 24-hour range, optional display unit for metrics, and an optional refresh interval. Builds, queue, and deployments display the connection's collected snapshot. Metric, log, and trace cards make explicit dashboard queries: once when the dashboard opens, again from their refresh button, and optionally at their selected interval while the dashboard remains open. Metric cards default to 60 seconds; log and trace cards default to manual refresh. Dashboard log queries return at most 100 rows and trace queries at most 30 summaries; they remain bounded to 24 hours.

Metric checks are user-defined threshold comparisons, not automated root-cause analysis. Presets require the corresponding metrics: node-exporter for CPU/memory/filesystems and kube-state-metrics for container restarts. Rules start disabled. Empty, stale, or failed results are not classified as healthy. One `OutOfSync` application can still be `Healthy`; both are shown.

CI/CD polls wait 15 seconds after a cycle; monitoring waits 30 seconds. A cycle is bounded to 60 seconds and at most two collection/query operations run concurrently. HTTP calls time out after 8 seconds and responses are limited to 8 MiB. Discovery is capped at 200 folders/2,000 jobs, saved selections at 50 targets and 20 rules per connection. Queries are limited to 24 hours, 100 returned series, and approximately 600 samples per series. Warnings and truncation are surfaced. These are guardrails for a small personal dashboard, not a high-volume ingestion service.

Collection continues while the app is running, including minimization. Closing the app stops it. There is no tray process or Windows service. Resume requests an immediate refresh; actual sleep/resume acceptance is still pending.

Metric nodes support Ctrl+wheel zoom over the node, or the graph's +/− buttons. Click the percentage to restore the full time range. Zoom changes only the horizontal time range around the pointer. The vertical value scale, graph height, and node dimensions stay unchanged; no extra query is issued. Zoom is temporary and resets when the dashboard is reopened or the query changes.

Metric graphs show three vertical-axis values and place the last visible sample's value immediately to the right at its plotted height, using the same 11px text styling as the axis labels. Hover over the graph to inspect each series' nearest sample time and value; gaps remain missing values. The old separate “최근 값” heading is removed.

API queries use embedded gojq; no jq installation is needed. Only JSON is supported. Strings and numbers appear as values; objects, arrays and multiple jq outputs appear as formatted JSON. Large integers remain exact text across the desktop bridge. HTTP requests have a 20-second timeout and an 8 MiB input limit; jq has a cooperative 2-second context and a 200-output/1 MiB display limit. Redirects and URL user-info are rejected, and credentials/cookies are not added. Do not put secrets in URLs: the URL and query are saved with the node. Query results stay in memory. Saving a node, opening the dashboard or pressing the global refresh button does not execute API requests; use the node's refresh button. Select **저장 · 배치 완료** after adding or editing a node to persist its configuration.

## Local archive and credentials

- Settings, favorites, and build summaries: `%LOCALAPPDATA%\IDPDashboard\dashboard.db`.
- The personal dashboard layout is the `dashboard.layout` version 1 preference in that same SQLite database; it does not add an archive or connection-schema migration. An intentionally empty board is saved as empty. An invalid or newer saved layout is left untouched and the dashboard stays read-only until it can be read again.
- Demo data: `%LOCALAPPDATA%\IDPDashboard\demo\dashboard.db`.
- Credentials: Windows Credential Manager, service `IDPDashboard`, one random reference per saved credential. Tokens are not returned to the frontend or included in SQLite backups.
- Archive records have no automatic expiry. Removing a connection keeps its build summaries. Monitor disk usage shown in the history view.
- Changing a provider or API URL requires a new connection so unrelated histories are not merged.

Builds are identified by connection, job, build number, and start timestamp. Completed statuses do not regress to running because of delayed observations. If an observed running build disappears, its retained record becomes `UNCONFIRMED`. Deleted history that the app never observed cannot be reconstructed. Historical pages are scanned while current builds continue to be polled, and rescanned after restart. Extremely fast history churn can still create gaps; this is not a guaranteed backup of Jenkins.

Use **백업 내보내기** to create a consistent `.db` snapshot while the app runs. Existing files are not overwritten. To restore, close the app, retain a copy of the current database and any `-wal`/`-shm` files, and replace the database in an otherwise empty app-data directory with the exported file. Use the same app version or a compatible newer one. Re-enter credentials on another Windows account or PC; exported references do not contain credentials. CA files also need to exist at their configured paths. Backups contain infrastructure names, URLs, labels, and build metadata; handle them accordingly.

## Build and test

Tested with Go 1.25.7, Node 24.13.0, npm 11.10.0, and Wails 2.11.0. Microsoft Edge is used by the browser tests. The build script pins the Wails CLI through `go run`; no global Wails installation is required.

```powershell
.\scripts\build.ps1
.\scripts\test.ps1
```

The first run downloads dependencies. The test script installs locked npm dependencies, builds frontend assets, runs Go tests/vet, and runs Playwright at 1440×1000 and 480×760. Live services are not needed for default tests.

Optional tests, explicitly enabled:

```powershell
# Read-only integration with a reachable unauthenticated Prometheus:
$env:IDP_TEST_PROMETHEUS_URL = 'http://localhost:9090'
go test ./internal/providers/prometheus -run TestPrometheusLive -v -count=1
Remove-Item Env:IDP_TEST_PROMETHEUS_URL

# Creates and removes only a temporary test credential in your account:
$env:IDP_TEST_KEYRING = '1'
go test . -run TestWindowsCredentialLifecycle -v -count=1
Remove-Item Env:IDP_TEST_KEYRING
```

For frontend-only development:

```powershell
Set-Location frontend
npm ci
npm run dev
```

The browser preview is an explicitly labelled in-memory fixture, with `/?empty` for onboarding. It does not access native settings or credentials. Desktop demo mode tests a different, fuller path through the Go adapters and database.

## Contribution and scope

See [ARCHITECTURE.md](ARCHITECTURE.md) for the package layout, dependency rules, and four collection contracts, [CONTRIBUTING.md](CONTRIBUTING.md) for adapter registration, and [VERIFICATION.md](VERIFICATION.md) for actual results and pending checks. OCI was the initial integration environment; its addresses are not compiled into the application.

Adapters live in `internal/providers/<name>` with their own metadata and tests, and are registered in `internal/providers/builtin.go`. They are contributed as source and compiled with the app. There is no runtime plugin loader. Providers compose independent build, queue, deployment, and metric capabilities; a single provider can supply several. Metric query languages and defaults come from provider metadata. Authentication protocols outside the implemented flows still require explicit implementation. The archive still requires numeric build IDs and offset pagination; opaque IDs/cursors have not yet been generalized. Cross-platform packaging, remote multi-user access, full log retention, and notification delivery are outside this version.

The existing `000.*`, `001.*`, and `002.*` files are design history. This README describes the implementation. A project license and public release process have not yet been selected; this working tree should not be presented as a licensed public release.
