// Package providers composes the adapters shipped with the application.
package providers

import (
	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/providers/argocd"
	"idp-dashboard/internal/providers/jenkins"
	"idp-dashboard/internal/providers/loki"
	"idp-dashboard/internal/providers/prometheus"
	"idp-dashboard/internal/providers/tempo"
)

// BuiltinDefinitions is the single list to update when contributing an adapter.
func BuiltinDefinitions() []platform.Definition {
	return []platform.Definition{jenkins.Definition(), argocd.Definition(), prometheus.Definition(), loki.Definition(), tempo.Definition()}
}

func DefaultRegistry() *platform.Registry {
	r, err := NewRegistry()
	if err != nil {
		panic(err)
	}
	return r
}

func NewRegistry(extensions ...platform.Definition) (*platform.Registry, error) {
	definitions := []platform.Definition{argocd.Definition(), prometheus.Definition(), loki.Definition(), tempo.Definition()}
	r, err := platform.NewRegistry(append(definitions, extensions...)...)
	if err != nil {
		return nil, err
	}
	if err := r.RegisterPlugin(jenkins.Manifest(), jenkins.Definition()); err != nil {
		return nil, err
	}
	return r, nil
}
