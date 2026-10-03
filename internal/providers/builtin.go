// Package providers composes the adapters shipped with the application.
package providers

import (
	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/providers/argocd"
	"idp-dashboard/internal/providers/jenkins"
	"idp-dashboard/internal/providers/prometheus"
)

// BuiltinDefinitions is the single list to update when contributing an adapter.
func BuiltinDefinitions() []platform.Definition {
	return []platform.Definition{jenkins.Definition(), argocd.Definition(), prometheus.Definition()}
}

func DefaultRegistry() *platform.Registry {
	r, err := platform.NewRegistry(BuiltinDefinitions()...)
	if err != nil {
		panic(err)
	}
	return r
}
