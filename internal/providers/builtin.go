// Package providers composes the adapters shipped with the application.
package providers

import (
	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/providers/argocd"
	"idp-dashboard/internal/providers/jenkins"
	"idp-dashboard/internal/providers/jsonapi"
	"idp-dashboard/internal/providers/loki"
	"idp-dashboard/internal/providers/prometheus"
	"idp-dashboard/internal/providers/tempo"
)

// BuiltinDefinitions is the single list to update when contributing an adapter.
func BuiltinDefinitions() []platform.Definition {
	return []platform.Definition{jenkins.Definition(), argocd.Definition(), prometheus.Definition(), loki.Definition(), tempo.Definition(), jsonapi.Definition()}
}

func DefaultRegistry() *platform.Registry {
	r, err := NewRegistry()
	if err != nil {
		panic(err)
	}
	return r
}

func NewRegistry(extensions ...platform.Definition) (*platform.Registry, error) {
	r, err := platform.NewRegistry(extensions...)
	if err != nil {
		return nil, err
	}
	plugins := []struct {
		manifest   sdk.Manifest
		definition platform.Definition
	}{
		{jsonapi.Manifest(), jsonapi.Definition()},
		{jenkins.Manifest(), jenkins.Definition()}, {argocd.Manifest(), argocd.Definition()},
		{prometheus.Manifest(), prometheus.Definition()}, {loki.Manifest(), loki.Definition()}, {tempo.Manifest(), tempo.Definition()},
	}
	for _, plugin := range plugins {
		if err = r.RegisterPlugin(plugin.manifest, plugin.definition); err != nil {
			return nil, err
		}
	}
	return r, nil
}
