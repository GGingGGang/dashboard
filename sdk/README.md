# Dashboard extension SDK (contract v1, unreleased)

This independent Go module contains the data contracts used by the dashboard
and provider implementations. It depends only on the Go standard library.
The application registry, collector, SQLite store, credentials and Wails bridge
are not part of this module. The `httpapi` subpackage is an optional transport
helper; its redirect, TLS, timeout, response-limit and error-redaction behavior
is the same as the former internal provider helper.

## Local development

The module path is `github.com/GGingGGang/dashboard/sdk`. No version has been
published. Consumers currently use a local replacement:

```go
require github.com/GGingGGang/dashboard/sdk v0.0.0
replace github.com/GGingGGang/dashboard/sdk => /path/to/dashboard/sdk
```

```go
import sdk "github.com/GGingGGang/dashboard/sdk"

func Definition() sdk.Definition {
    return sdk.Definition{
        Info: sdk.ProviderInfo{
            Kind: "example", Name: "Example", DefaultAuth: "none",
            AuthMethods: []string{"none"}, Capabilities: []string{"metrics"},
            Query: &sdk.QueryInfo{Language: "ExampleQL"},
        },
        Create: func(c sdk.Connection, secret string) (sdk.Provider, error) {
            return newExampleClient(c, secret) // implements Check and Query
        },
    }
}
```

Implement only supported capability interfaces. `TargetDiscoverer` is optional;
targets describe a query scope, not an individual query expression. Keep opaque
IDs and nanosecond timestamps as strings and numeric gaps as nil values. The host
retains copies of returned data, but a provider must not mutate a result
concurrently with its return/serialization. Credentials are provided only when
constructing a configured client, not while reading metadata.

## Extension declarations

`Manifest` declares providers, cards, dashboards and query contributions using
host-rendered UI. `ContractVersion` identifies the extension contract separately
from package and upstream API versions. Schemas are JSON documents, not Go
callbacks or executable frontend code.

`SourceRegistrar` is a host-supplied, provider-scoped capability to register,
replace or remove local source registrations. Providers never import the core
registry or storage. `SourcePublisher` is an optional publication hook; providers
without it retain the existing typed capabilities. Updates and deletion refer to
registrations only, not upstream resources or archived history.

These extension declarations are the first migration step. The current app
still uses its compiled registration list: it does **not yet** invoke
`SourcePublisher`, load manifests, execute external packages, or render contributed
UI. Runtime validation and lifecycle implementation belong to the next step.
This unreleased contract may change with that implementation.

## Verification

Run `go test ./...` and `go vet ./...` from this directory. The application also
tests all five shipped providers by copying their production sources into an
external consumer module with only this SDK dependency and network resolution
disabled. Existing root integration tests continue to exercise the registry,
collector, HTTP fixtures and database.
