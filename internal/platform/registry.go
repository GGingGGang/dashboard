package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"sync"

	sdk "github.com/GGingGGang/dashboard/sdk"
)

type Registry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
	owners      map[string]string
	plugins     map[string]PluginState
}

func NewRegistry(definitions ...Definition) (*Registry, error) {
	r := &Registry{definitions: map[string]Definition{}, owners: map[string]string{}, plugins: map[string]PluginState{}}
	for _, d := range definitions {
		m := sdk.Manifest{ID: d.Info.Kind, Version: "0.1.0", ContractVersion: sdk.ContractVersion, Providers: []sdk.ProviderContribution{{ID: d.Info.Kind, Name: d.Info.Name}}}
		if err := r.RegisterPlugin(m, d); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func normalizeDefinitions(definitions []Definition) (map[string]Definition, error) {
	normalized := map[string]Definition{}
	for _, d := range definitions {
		if d.Info.Kind == "" || d.Create == nil {
			return nil, errors.New("Provider requires a kind and constructor")
		}
		if _, exists := normalized[d.Info.Kind]; exists {
			return nil, fmt.Errorf("Duplicate provider: %s", d.Info.Kind)
		}
		if d.Info.PollSeconds <= 0 {
			d.Info.PollSeconds = 15
		}
		if !slices.Contains(d.Info.AuthMethods, d.Info.DefaultAuth) {
			return nil, errors.New("Default authentication must be supported")
		}
		d.Info = d.Info.Clone()
		if d.Info.Queries == nil {
			d.Info.Queries = map[string]QueryInfo{}
		}
		if d.Info.Query == nil {
			if q, ok := d.Info.Queries["metrics"]; ok {
				d.Info.Query = &q
			}
		}
		if d.Info.Query != nil {
			d.Info.Queries["metrics"] = *d.Info.Query
		}
		if slices.Contains(d.Info.Capabilities, "metrics") && (d.Info.Query == nil || d.Info.Query.Language == "") {
			return nil, errors.New("Metrics provider requires query metadata")
		}
		for _, capability := range []string{"logs", "traces"} {
			if slices.Contains(d.Info.Capabilities, capability) && d.Info.Queries[capability].Language == "" {
				return nil, fmt.Errorf("%s provider requires query metadata", capability)
			}
		}
		d.Info = d.Info.Clone()
		normalized[d.Info.Kind] = d
	}
	return normalized, nil
}

func (r *Registry) Infos() []ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ProviderInfo, 0, len(r.definitions))
	for _, d := range r.definitions {
		out = append(out, d.Info.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) Info(kind string) (ProviderInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.definitions[kind]
	return d.Info.Clone(), ok
}

func (r *Registry) definition(kind string) (Definition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.definitions[kind]
	if !ok {
		return d, errors.New("Unknown provider")
	}
	if !r.plugins[r.owners[kind]].Enabled {
		return d, errors.New("Plugin is disabled")
	}
	d.Info = d.Info.Clone()
	return d, nil
}

func (r *Registry) Validate(c Connection) error {
	d, err := r.definition(c.Kind)
	if err != nil {
		return err
	}
	if _, err := r.configured(c); err != nil {
		return err
	}
	return validateDefinition(d, c)
}

func validateDefinition(d Definition, c Connection) error {
	if err := ValidateConnection(c); err != nil {
		return err
	}
	if !slices.Contains(d.Info.AuthMethods, c.Auth) {
		return errors.New("이 플랫폼에서 지원하지 않는 인증 방식입니다. 연결 설정에서 인증 방식을 다시 선택하세요")
	}
	if validate := d.Validate; validate != nil {
		return validate(c.Clone())
	}
	return nil
}

func (r *Registry) New(c Connection, secret string) (Provider, error) {
	d, err := r.definition(c.Kind)
	if err != nil {
		return nil, err
	}
	c, err = r.configured(c)
	if err != nil {
		return nil, err
	}
	if err := validateDefinition(d, c); err != nil {
		return nil, err
	}
	if c.Auth != "none" && secret == "" {
		return nil, errors.New("Authentication secret is missing")
	}
	p, err := d.Create(c.Clone(), secret)
	if err != nil {
		return nil, err
	}
	if err := ValidateCapabilities(p, SourceCapabilities(d.Info.Capabilities)); err != nil {
		if !nilProvider(p) {
			_ = CloseProvider(p)
		}
		return nil, err
	}
	return p, nil
}

// Historical ProviderInfo also contains feature labels that are not Go source
// interfaces. Preserve those for existing UI consumers, excluding only the
// documented compatibility labels from source registration and validation.
func SourceCapabilities(capabilities []string) []string {
	out := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		if !slices.Contains([]string{"history", "sync", "health", "instant", "range", "rules"}, capability) {
			out = append(out, capability)
		}
	}
	return out
}

func nilProvider(p Provider) bool {
	if p == nil {
		return true
	}
	v := reflect.ValueOf(p)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Interface, reflect.Chan:
		return v.IsNil()
	}
	return false
}

// ValidateCapabilities compares exact implemented contracts, not product names.
func ValidateCapabilities(p Provider, declared []string) error {
	if nilProvider(p) {
		return errors.New("Provider constructor returned no provider")
	}
	actual := []string{}
	if _, ok := sdk.Resolve(p, "json").(sdk.JSONSource); ok {
		actual = append(actual, "json")
	}
	if _, ok := sdk.Resolve(p, "data").(sdk.DataSource); ok {
		actual = append(actual, "data")
	}
	if _, ok := sdk.Resolve(p, "builds").(BuildSource); ok {
		actual = append(actual, "builds")
	}
	if _, ok := sdk.Resolve(p, "queue").(QueueSource); ok {
		actual = append(actual, "queue")
	}
	if _, ok := sdk.Resolve(p, "deployments").(CD); ok {
		actual = append(actual, "deployments")
	}
	if _, ok := sdk.Resolve(p, "metrics").(Monitoring); ok {
		actual = append(actual, "metrics")
	}
	if _, ok := sdk.Resolve(p, "logs").(LogSource); ok {
		actual = append(actual, "logs")
	}
	if _, ok := sdk.Resolve(p, "traces").(TraceSource); ok {
		actual = append(actual, "traces")
	}
	if len(actual) == 0 {
		return errors.New("Provider implements no collection capability")
	}
	for _, capability := range declared {
		if !slices.Contains(actual, capability) {
			return fmt.Errorf("Provider capability mismatch: %s", capability)
		}
	}
	for _, capability := range actual {
		if !slices.Contains(declared, capability) {
			return fmt.Errorf("Provider capability mismatch: %s", capability)
		}
	}
	if len(declared) != len(actual) {
		return errors.New("Duplicate provider capability")
	}
	return nil
}

func (r *Registry) configured(c Connection) (Connection, error) {
	r.mu.RLock()
	plugin := r.plugins[r.owners[c.Kind]]
	r.mu.RUnlock()
	for _, p := range plugin.Manifest.Providers {
		if p.ID == c.Kind {
			schema, err := sdk.ParseFormSchema(p.ConfigSchema)
			if err != nil {
				return c, err
			}
			values, err := schema.Values(c.Config)
			if err != nil {
				return c, err
			}
			c.Config, err = json.Marshal(values)
			return c, err
		}
	}
	return c, errors.New("Provider declaration missing")
}
