package protocol

import sdk "github.com/GGingGGang/dashboard/sdk"

type Initialize struct {
	Version    int            `json:"version"`
	Kind       string         `json:"kind"`
	Connection sdk.Connection `json:"connection"`
	Secret     string         `json:"secret"`
}
type Initialized struct {
	Version      int      `json:"version"`
	Capabilities []string `json:"capabilities"`
	Discover     bool     `json:"discover"`
}
type SourceChange struct {
	Descriptor sdk.SourceDescriptor `json:"descriptor"`
	Handle     string               `json:"handle"`
}
type Request struct {
	JSON   sdk.JSONRequest   `json:"json"`
	Handle string            `json:"handle"`
	Target string            `json:"target"`
	Offset int               `json:"offset"`
	Number int64             `json:"number"`
	Query  sdk.Query         `json:"query"`
	Search sdk.SearchRequest `json:"search"`
	Input  map[string]any    `json:"input"`
}
type Builds struct {
	Items []sdk.Build `json:"items"`
	More  bool        `json:"more"`
}

func Capabilities(p sdk.Provider) []string {
	out := []string{}
	if _, ok := sdk.Resolve(p, "json").(sdk.JSONSource); ok {
		out = append(out, "json")
	}
	if _, ok := sdk.Resolve(p, "builds").(sdk.BuildSource); ok {
		out = append(out, "builds")
	}
	if _, ok := sdk.Resolve(p, "queue").(sdk.QueueSource); ok {
		out = append(out, "queue")
	}
	if _, ok := sdk.Resolve(p, "deployments").(sdk.DeploymentSource); ok {
		out = append(out, "deployments")
	}
	if _, ok := sdk.Resolve(p, "metrics").(sdk.MetricSource); ok {
		out = append(out, "metrics")
	}
	if _, ok := sdk.Resolve(p, "logs").(sdk.LogSource); ok {
		out = append(out, "logs")
	}
	if _, ok := sdk.Resolve(p, "traces").(sdk.TraceSource); ok {
		out = append(out, "traces")
	}
	if _, ok := sdk.Resolve(p, "data").(sdk.DataSource); ok {
		out = append(out, "data")
	}
	return out
}
