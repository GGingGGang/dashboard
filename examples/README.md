# Connection examples

These are UI configuration examples, not an import format. Replace the sample domains with addresses reachable from the Windows PC. Never put tokens into this directory.

| Field | Jenkins | Argo CD | Prometheus |
| --- | --- | --- | --- |
| API URL | `https://ci.example.org/jenkins` | `https://deploy.example.org` | `https://metrics.example.org/prometheus` |
| Authentication | Basic | Bearer or Argo CD login | None, Basic, or Bearer as configured upstream |
| Username | A user with read access | Local account name for login; empty for bearer | Only when Basic is selected |
| Secret | Jenkins API token | API token for bearer; local-account password for login | Proxy/API credential if required |
| Selection | Discover and select branch jobs | Discover and select applications | Enable only rules with available metrics |
| Service/environment | e.g. `payments` / `production` | Use matching labels for related targets | Express scope in PromQL labels |

The API URL and optional browser URL are separate. For example, a PC may call a private API route while source links open a public HTTPS ingress. The app never derives a Kubernetes service address from a service name.

## OCI reference environment

The original `oci-always-free-k8s` environment exposes the three tools inside Kubernetes. Before configuring the app, establish PC-to-service connectivity through your existing approved network route or port-forward. Use the corresponding reachable URL and a read-only credential. No OCI-specific code or Kubernetes credential is needed by the dashboard.

The default node CPU/memory/filesystem rules assume node-exporter metrics. The container restart rule assumes kube-state-metrics. Labels can vary by installation, so execute each expression in **지표 탐색** first, verify its scope and units, then enable its threshold rule. `1 - up` checks scrape reachability; it does not prove application health.

## Reproducible local demonstration

Run `idp-dashboard.exe --demo`. The embedded example server listens only on a random loopback port and simulates all three APIs. Two jobs provide 160 build summaries each, applications demonstrate separate Sync/Health values, and Prometheus fixtures supply numeric query results. Restarting can create another set of synthetic timestamps; demo history has the same no-expiry policy as normal history.

No real Jenkins/Argo/Prometheus deployment is provisioned by the demo. Use the live acceptance checklist in `VERIFICATION.md` before claiming compatibility with a particular installation.
