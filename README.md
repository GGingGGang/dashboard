# IDP Dashboard

A Windows desktop dashboard for build history, deployment status, and PromQL checks. Connect to your existing tools by URL and read-only credentials. No dashboard server, Kubernetes controller, or OCI account is required.

This is an early implementation of an operations view for a developer platform, not a complete internal developer platform. The interface is Korean; contributor documentation is English.

## Run

Requirements: Windows x64 and Microsoft Edge WebView2 Runtime. The executable embeds the frontend.

```powershell
.\build\bin\idp-dashboard.exe
# Separate demo database and synthetic loopback APIs:
.\build\bin\idp-dashboard.exe --demo
```

In **연결 설정 → 연결 추가**, select a provider, enter its base URL and authentication, then choose **연결 검사 · 대상 찾기**. Select jobs or applications to watch and save. Prometheus connections instead have optional query rules. Multiple connections of each provider are supported.

Demo mode exercises the same Go HTTP adapters and SQLite collector as real connections. Its sample data is explicitly labelled. It creates a separate database and never connects to your infrastructure unless you manually add a real connection. Demo connections are reset on each launch.

## Current integrations

| Provider | Read operations | Configuration |
| --- | --- | --- |
| Jenkins | Folder/multibranch discovery, build summaries, queue, historical backfill | Controller base URL, typically Basic auth with username/API token |
| Argo CD | Application discovery, Sync, Health, revision, last operation | Server base URL and bearer token, or explicit local-account session login |
| Prometheus | Instant and range queries, threshold rules | Prometheus base URL; none, Basic, or bearer authentication |

Resource requests are read-only. Explicit Argo CD account login uses `POST /api/v1/session` to obtain a session token; the app cannot trigger builds, sync applications, or change monitoring configuration. Current integrations use existing HTTP APIs and need no upstream plugin. Authentication and network access still have to be configured on each upstream service.

For Argo CD, choose **Bearer · API 토큰** for an existing token, or **Argo CD 로그인 · 사용자명 + 비밀번호** for a local account such as `admin`. HTTP Basic is not Argo CD account login. The login option stores the password in Windows Credential Manager, keeps the issued token in memory, and retries an expired session once. Rejected login credentials stop automatic login attempts until the connection is saved again. SSO is not supported. Existing Basic connections require selecting a supported method and re-entering the credential; credentials are not silently reinterpreted. See the [Argo CD API documentation](https://argo-cd.readthedocs.io/en/stable/developer-guide/api-docs/).

Supply a **base URL**, including a reverse-proxy prefix if applicable; do not append `/api/json` or `/api/v1`. Jenkins requires its controller root, not a job/folder URL. An optional browser URL allows source links to use another address. An optional PEM CA file extends the system trust store. TLS verification is always enabled; redirects are rejected to avoid forwarding credentials to another endpoint. Configure the final URL directly.

Connections are made from the PC. A private Kubernetes ClusterIP requires an existing route, VPN, or explicitly managed port-forward. The app does not create these. See [configuration examples](examples/README.md).

## Views and interpretation

- **운영 현황**: latest failed completed build per job, selected queue items, independent deployment Sync/Health, metric rules, and source freshness. Service/environment labels map selected CI/CD targets; metric queries retain their own label scope.
- **빌드 이력**: local build summaries, filters, pagination, and consistent SQLite backup. Console logs and artifacts are not archived.
- **지표 탐색**: PromQL instant/range queries, charts, values, and query favorites.
- **연결 설정**: multiple instances, credentials, target discovery, service/environment mapping, and editable rules.

Metric checks are user-defined threshold comparisons, not automated root-cause analysis. Presets require the corresponding metrics: node-exporter for CPU/memory/filesystems and kube-state-metrics for container restarts. Rules start disabled. Empty, stale, or failed results are not classified as healthy. One `OutOfSync` application can still be `Healthy`; both are shown.

CI/CD polls wait 15 seconds after a cycle; monitoring waits 30 seconds. A cycle is bounded to 60 seconds and at most two collection/query operations run concurrently. HTTP calls time out after 8 seconds and responses are limited to 8 MiB. Discovery is capped at 200 folders/2,000 jobs, saved selections at 50 targets and 20 rules per connection. Queries are limited to 24 hours, 100 returned series, and approximately 600 samples per series. Warnings and truncation are surfaced. These are guardrails for a small personal dashboard, not a high-volume ingestion service.

Collection continues while the app is running, including minimization. Closing the app stops it. There is no tray process or Windows service. Resume requests an immediate refresh; actual sleep/resume acceptance is still pending.

## Local archive and credentials

- Settings, favorites, and build summaries: `%LOCALAPPDATA%\IDPDashboard\dashboard.db`.
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
go test ./internal/platform -run TestPrometheusLive -v -count=1
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

See [ARCHITECTURE.md](ARCHITECTURE.md) for the four collection modules and their data contracts, [CONTRIBUTING.md](CONTRIBUTING.md) for adapter registration, and [VERIFICATION.md](VERIFICATION.md) for actual results and pending checks. OCI was the initial integration environment; its addresses are not compiled into the application.

Adapters are contributed as source and compiled with the app. There is no runtime plugin loader. Providers compose independent build, queue, deployment, and metric capabilities; a single provider can supply several. Metric query languages and defaults come from provider metadata. Authentication protocols outside the implemented flows still require explicit implementation. The archive still requires numeric build IDs and offset pagination; opaque IDs/cursors have not yet been generalized. Cross-platform packaging, remote multi-user access, full log retention, and notification delivery are outside this version.

The existing `000.*`, `001.*`, and `002.*` files are design history. This README describes the implementation. A project license and public release process have not yet been selected; this working tree should not be presented as a licensed public release.
