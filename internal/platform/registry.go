package platform

import (
	"errors"
	"fmt"
	"slices"
	"sort"
)

// Definition is the only registration a new provider needs. The factory owns
// protocol-specific authentication and translates APIs into capability contracts.
type Definition struct {
	Info   ProviderInfo
	Create func(Connection, string) (Provider, error)
}

type Registry struct{ definitions map[string]Definition }

func NewRegistry(definitions ...Definition) (*Registry, error) {
	r := &Registry{definitions: map[string]Definition{}}
	for _, d := range definitions {
		if d.Info.Kind == "" || d.Create == nil {
			return nil, errors.New("Provider requires a kind and constructor")
		}
		if _, exists := r.definitions[d.Info.Kind]; exists {
			return nil, fmt.Errorf("Duplicate provider: %s", d.Info.Kind)
		}
		if d.Info.PollSeconds <= 0 {
			d.Info.PollSeconds = 15
		}
		if !slices.Contains(d.Info.AuthMethods, d.Info.DefaultAuth) {
			return nil, errors.New("Default authentication must be supported")
		}
		if slices.Contains(d.Info.Capabilities, "metrics") && (d.Info.Query == nil || d.Info.Query.Language == "") {
			return nil, errors.New("Metrics provider requires query metadata")
		}
		r.definitions[d.Info.Kind] = d
	}
	return r, nil
}

func (r *Registry) Infos() []ProviderInfo {
	out := make([]ProviderInfo, 0, len(r.definitions))
	for _, d := range r.definitions {
		out = append(out, d.Info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) Info(kind string) (ProviderInfo, bool) {
	d, ok := r.definitions[kind]
	return d.Info, ok
}

func (r *Registry) Validate(c Connection) error {
	if err := validateConnection(c); err != nil {
		return err
	}
	info, ok := r.Info(c.Kind)
	if !ok {
		return errors.New("Unknown provider")
	}
	if !slices.Contains(info.AuthMethods, c.Auth) {
		return errors.New("이 플랫폼에서 지원하지 않는 인증 방식입니다. 연결 설정에서 인증 방식을 다시 선택하세요")
	}
	return nil
}

func (r *Registry) New(c Connection, secret string) (Provider, error) {
	if err := r.Validate(c); err != nil {
		return nil, err
	}
	if c.Auth != "none" && secret == "" {
		return nil, errors.New("Authentication secret is missing")
	}
	p, err := r.definitions[c.Kind].Create(c, secret)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("Provider constructor returned no provider")
	}
	actual := []string{}
	if _, ok := p.(BuildSource); ok {
		actual = append(actual, "builds")
	}
	if _, ok := p.(QueueSource); ok {
		actual = append(actual, "queue")
	}
	if _, ok := p.(CD); ok {
		actual = append(actual, "deployments")
	}
	if _, ok := p.(Monitoring); ok {
		actual = append(actual, "metrics")
	}
	if len(actual) == 0 {
		return nil, errors.New("Provider implements no collection capability")
	}
	info, _ := r.Info(c.Kind)
	for _, capability := range []string{"builds", "queue", "deployments", "metrics"} {
		if slices.Contains(info.Capabilities, capability) != slices.Contains(actual, capability) {
			return nil, fmt.Errorf("Provider capability mismatch: %s", capability)
		}
	}
	return p, nil
}
