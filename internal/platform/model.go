package platform

import (
	"context"
	"time"
)

type Connection struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	BrowserURL string   `json:"browserUrl"`
	Auth       string   `json:"auth"`
	Username   string   `json:"username"`
	CAFile     string   `json:"caFile"`
	Targets    []Target `json:"targets"`
	Rules      []Rule   `json:"rules"`
}

type Target struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Service     string `json:"service"`
	Environment string `json:"environment"`
}

type ProviderInfo struct {
	Kind         string   `json:"kind"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	Capabilities []string `json:"capabilities"`
	DefaultAuth  string   `json:"defaultAuth"`
}

type Provider interface {
	Check(context.Context) error
	Discover(context.Context) ([]Target, error)
}

type CI interface {
	Provider
	Builds(context.Context, string, int) ([]Build, bool, error)
	Build(context.Context, string, int64) (Build, error)
	Queue(context.Context) ([]QueueItem, error)
}

type CD interface {
	Provider
	Deployments(context.Context) ([]Deployment, error)
}

type Monitoring interface {
	Provider
	Query(context.Context, Query) (QueryResult, error)
}

type Build struct {
	ConnectionID string `json:"connectionId"`
	Job          string `json:"job"`
	Number       int64  `json:"number"`
	Started      int64  `json:"started"`
	Duration     int64  `json:"duration"`
	Status       string `json:"status"`
	RawStatus    string `json:"rawStatus"`
	Commit       string `json:"commit"`
	URL          string `json:"url"`
	Observed     int64  `json:"observed"`
}

type QueueItem struct {
	ID     int64  `json:"id"`
	Job    string `json:"job"`
	Since  int64  `json:"since"`
	Reason string `json:"reason"`
	URL    string `json:"url"`
}

type Deployment struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Project   string   `json:"project"`
	Sync      string   `json:"sync"`
	Health    string   `json:"health"`
	Revisions []string `json:"revisions"`
	Phase     string   `json:"phase"`
	Message   string   `json:"message"`
	Finished  string   `json:"finished"`
	URL       string   `json:"url"`
}

type Query struct {
	Expression string `json:"expression"`
	Start      int64  `json:"start"`
	End        int64  `json:"end"`
}

type Point struct {
	Time  float64  `json:"time"`
	Value *float64 `json:"value"`
}

type Series struct {
	Labels map[string]string `json:"labels"`
	Points []Point           `json:"points"`
}

type QueryResult struct {
	Type      string   `json:"type"`
	Series    []Series `json:"series"`
	Warnings  []string `json:"warnings"`
	Truncated bool     `json:"truncated"`
}

type Rule struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Expression  string  `json:"expression"`
	Threshold   float64 `json:"threshold"`
	Unit        string  `json:"unit"`
	Description string  `json:"description"`
	Enabled     bool    `json:"enabled"`
}

type RuleResult struct {
	Rule     Rule        `json:"rule"`
	Result   QueryResult `json:"result"`
	Breaches int         `json:"breaches"`
	Error    string      `json:"error"`
}

type Snapshot struct {
	ConnectionID    string       `json:"connectionId"`
	Attempted       time.Time    `json:"attempted"`
	LastSuccess     time.Time    `json:"lastSuccess"`
	Error           string       `json:"error"`
	StorageError    string       `json:"storageError"`
	Builds          []Build      `json:"builds"`
	Queue           []QueueItem  `json:"queue"`
	Deployments     []Deployment `json:"deployments"`
	Rules           []RuleResult `json:"rules"`
	Imported        int          `json:"imported"`
	BackfillPending bool         `json:"backfillPending"`
}

func Presets() []Rule {
	return []Rule{
		{ID: "cpu", Name: "Node CPU", Expression: `100 * (1 - avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])))`, Threshold: 85, Unit: "%", Description: "5-minute average; requires node-exporter"},
		{ID: "memory", Name: "Node memory", Expression: `100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)`, Threshold: 90, Unit: "%", Description: "Available memory ratio; requires node-exporter"},
		{ID: "disk", Name: "Filesystem usage", Expression: `100 * (1 - node_filesystem_avail_bytes{fstype!~"tmpfs|overlay|squashfs",mountpoint!~"/run.*"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay|squashfs",mountpoint!~"/run.*"})`, Threshold: 85, Unit: "%", Description: "Per filesystem; requires node-exporter"},
		{ID: "restarts", Name: "Container restarts", Expression: `increase(kube_pod_container_status_restarts_total[15m])`, Threshold: 2.999, Unit: "restarts", Description: "3 or more restarts in 15 minutes; requires kube-state-metrics"},
		{ID: "targets", Name: "Scrape failures", Expression: `1 - up`, Threshold: 0, Unit: "", Description: "1 means the target is down; 0 means reachable"},
	}
}
