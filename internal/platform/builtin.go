package platform

// BuiltinDefinitions is the composition point. Core collection and persistence
// depend on contracts, not these concrete providers.
func BuiltinDefinitions() []Definition {
	wrap := func(makeProvider func(*api) Provider) func(Connection, string) (Provider, error) {
		return func(c Connection, secret string) (Provider, error) {
			a, err := newAPI(c, secret)
			if err != nil {
				return nil, err
			}
			return makeProvider(a), nil
		}
	}
	return []Definition{
		{Info: ProviderInfo{Kind: "jenkins", Name: "Jenkins", Category: "ci", Capabilities: []string{"builds", "queue", "history"}, DefaultAuth: "basic", AuthMethods: []string{"basic", "bearer", "none"}, PollSeconds: 15}, Create: wrap(func(a *api) Provider { return &Jenkins{a} })},
		{Info: ProviderInfo{Kind: "argocd", Name: "Argo CD", Category: "cd", Capabilities: []string{"deployments", "sync", "health"}, DefaultAuth: "bearer", AuthMethods: []string{"bearer", "argocd-login", "none"}, PollSeconds: 15}, Create: wrap(func(a *api) Provider { return &ArgoCD{api: a} })},
		{Info: ProviderInfo{Kind: "prometheus", Name: "Prometheus", Category: "monitoring", Capabilities: []string{"metrics", "instant", "range", "rules"}, DefaultAuth: "none", AuthMethods: []string{"none", "basic", "bearer"}, PollSeconds: 30, Query: &QueryInfo{Language: "PromQL", DefaultExpression: "up", Presets: Presets()}}, Create: wrap(func(a *api) Provider { return &Prometheus{a} })},
	}
}

func DefaultRegistry() *Registry {
	r, err := NewRegistry(BuiltinDefinitions()...)
	if err != nil {
		panic(err)
	}
	return r
}

// Convenience entry points for existing callers.
func New(c Connection, secret string) (Provider, error) { return DefaultRegistry().New(c, secret) }
func Validate(c Connection) error                       { return DefaultRegistry().Validate(c) }
func Providers() []ProviderInfo                         { return DefaultRegistry().Infos() }
