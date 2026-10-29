package sdk

// CapabilityResolver allows a transport-backed provider to expose exactly the
// interfaces declared by its remote implementation. In-process adapters need not implement it.
type CapabilityResolver interface{ Capability(string) Provider }

func Resolve(p Provider, capability string) Provider {
	if r, ok := p.(CapabilityResolver); ok {
		return r.Capability(capability)
	}
	return p
}

// ProviderStatus reports a terminal transport failure without performing I/O.
type ProviderStatus interface{ Err() error }
