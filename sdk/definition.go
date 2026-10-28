package sdk

// Definition is the only registration a new provider needs. The factory owns
// protocol-specific authentication and translates APIs into capability contracts.
type Definition struct {
	Info     ProviderInfo
	Validate func(Connection) error
	Create   func(Connection, string) (Provider, error)
}
