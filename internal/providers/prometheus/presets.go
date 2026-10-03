package prometheus

import "idp-dashboard/internal/platform"

// Presets provides the built-in Prometheus queries; other providers supply their own QueryInfo.
func Presets() []platform.Rule {
	return []platform.Rule{
		{ID: "cpu", Name: "Node CPU", Expression: `100 * (1 - avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])))`, Threshold: 85, Unit: "%", Description: "5-minute average; requires node-exporter"},
		{ID: "memory", Name: "Node memory", Expression: `100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)`, Threshold: 90, Unit: "%", Description: "Available memory ratio; requires node-exporter"},
		{ID: "disk", Name: "Filesystem usage", Expression: `100 * (1 - node_filesystem_avail_bytes{fstype!~"tmpfs|overlay|squashfs",mountpoint!~"/run.*"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay|squashfs",mountpoint!~"/run.*"})`, Threshold: 85, Unit: "%", Description: "Per filesystem; requires node-exporter"},
		{ID: "restarts", Name: "Container restarts", Expression: `increase(kube_pod_container_status_restarts_total[15m])`, Threshold: 2.999, Unit: "restarts", Description: "3 or more restarts in 15 minutes; requires kube-state-metrics"},
		{ID: "targets", Name: "Scrape failures", Expression: `1 - up`, Threshold: 0, Unit: "", Description: "1 means the target is down; 0 means reachable"},
	}
}
