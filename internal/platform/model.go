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
	Capabilities []string `json:"capabilities,omitempty"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Service      string   `json:"service"`
	Environment  string   `json:"environment"`
}

type ProviderInfo struct {
	Kind         string     `json:"kind"`
	Name         string     `json:"name"`
	Category     string     `json:"category"`
	Capabilities []string   `json:"capabilities"`
	DefaultAuth  string     `json:"defaultAuth"`
	AuthMethods  []string   `json:"authMethods"`
	Query        *QueryInfo `json:"query,omitempty"`
	PollSeconds  int        `json:"pollSeconds"`
}

type QueryInfo struct {
	Language          string `json:"language"`
	DefaultExpression string `json:"defaultExpression"`
	Presets           []Rule `json:"presets"`
}

// Capabilities are independently implemented and may coexist on one provider.
type BuildSource interface {
	Provider
	Builds(context.Context, string, int) ([]Build, bool, error)
	Build(context.Context, string, int64) (Build, error)
}

type QueueSource interface {
	Provider
	Queue(context.Context) ([]QueueItem, error)
}

type Provider interface {
	Check(context.Context) error
	Discover(context.Context) ([]Target, error)
}

type CI interface {
	BuildSource
	QueueSource
}

type DeploymentSource interface {
	Provider
	Deployments(context.Context) ([]Deployment, error)
}

type MetricSource interface {
	Provider
	Query(context.Context, Query) (QueryResult, error)
}

// Compatibility names for existing adapters. New adapters use capability names.
type CD = DeploymentSource
type Monitoring = MetricSource

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
	Modules         map[string]CollectionStatus `json:"modules"`
	ConnectionID    string                      `json:"connectionId"`
	Attempted       time.Time                   `json:"attempted"`
	LastSuccess     time.Time                   `json:"lastSuccess"`
	Error           string                      `json:"error"`
	StorageError    string                      `json:"storageError"`
	Builds          []Build                     `json:"builds"`
	Queue           []QueueItem                 `json:"queue"`
	Deployments     []Deployment                `json:"deployments"`
	Rules           []RuleResult                `json:"rules"`
	Imported        int                         `json:"imported"`
	BackfillPending bool                        `json:"backfillPending"`
}

type CollectionStatus struct {
	Attempted   time.Time `json:"attempted"`
	LastSuccess time.Time `json:"lastSuccess"`
	Error       string    `json:"error"`
}
