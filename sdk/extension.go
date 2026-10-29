package sdk

import (
	"context"
	"encoding/json"
)

// ContractVersion versions the host/extension contract, not an upstream API.
const ContractVersion = 1

// Manifest contains declarations only. Reading it must not execute a provider.
// UI contributions use host renderers; arbitrary scripts are not supported.
type Manifest struct {
	ID              string                  `json:"id"`
	Version         string                  `json:"version"`
	ContractVersion int                     `json:"contractVersion"`
	Providers       []ProviderContribution  `json:"providers"`
	Cards           []CardContribution      `json:"cards"`
	Dashboards      []DashboardContribution `json:"dashboards"`
	Queries         []QueryContribution     `json:"queries"`
}

type ProviderContribution struct {
	// Info supplies metadata without executing an external binary.
	Info         *ProviderInfo   `json:"info,omitempty"`
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Executable   string          `json:"executable"`
	ConfigSchema json.RawMessage `json:"configSchema"`
}

type CardContribution struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Capability   string          `json:"capability"`
	Renderer     string          `json:"renderer"`
	ConfigSchema json.RawMessage `json:"configSchema"`
}

type DashboardContribution struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Cards []string `json:"cards"`
}

type QueryContribution struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Capability  string          `json:"capability"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Query       QueryInfo       `json:"query"`
	Renderer    string          `json:"renderer"`
}

// SourceDescriptor identifies one queryable scope, not one query expression.
// IDs are stable within the provider instance. The host assigns ownership.
type SourceDescriptor struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	TargetID     string   `json:"targetId,omitempty"`
	Capabilities []string `json:"capabilities"`
}

// SourceRegistrar is injected by the host, scoped to one provider instance.
// It manages local registrations only, never upstream resources or archives.
// Implementations validate capability declarations and copy descriptors before
// returning. Update replaces a registration atomically after validation.
// The host owns accepted implementations and closes them after their last call;
// a rejected implementation remains owned by the caller. A provider instance
// registered for multiple sources must be closed only after all uses end.
// In-process implementations use non-nil pointers for stable resource identity.
// After the final registration is removed, do not re-submit that implementation:
// its ownership has ended. Construct a new instance for subsequent registrations.
// A source ID must not be reassigned to a different TargetID within the scope.
type SourceRegistrar interface {
	Register(context.Context, SourceDescriptor, Provider) error
	Update(context.Context, SourceDescriptor, Provider) error
	Delete(context.Context, string) error
}

// SourcePublisher is optional. The host invokes it after configuring a provider.
// Providers without it continue to expose their existing capability interfaces.
// Publication must honor cancellation; it is not a long-running polling loop.
type SourcePublisher interface {
	PublishSources(context.Context, SourceRegistrar) error
}
