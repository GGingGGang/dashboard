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

The core now validates plugin identity, contract version and provider-definition
ownership before atomic registration. It persists enabled/disabled choices and
cancels affected connections when disabled. The scoped source registry implements
register/update/delete with capability validation, revision cancellation and
shared implementation lifetime. The host initializes `SourcePublisher` lazily on
`Sources`, `QuerySource`, `ExecuteContribution`, or opted-in collection calls; `default` is initially registered by the
host for the configured provider. Source implementations must be non-nil pointers.
After their final registration is removed, construct new instances rather than
re-submitting an implementation whose ownership has ended. Source IDs cannot be
reassigned to a different target within a scope.

`ProviderInfo.CollectionSource` opts collection into a registered source.
Jenkins publishes and collects `default`. Replacement cancels existing leases,
wakes collection and resets transient cursors; persisted history and connection
identity remain unchanged. Other collectors retain compatibility paths.

## Declarative UI and new data domains (phase 4)

The host reads Cards, Dashboards and Queries to build navigation, source selectors,
input forms and results. Dashboards reference card IDs from their manifest. Cards
and queries can be added to the personal board. Contributions may read any selected
source with the required capability, including another plugin's provider. Build
queries require a selected job from the saved connection.

Renderers: `table`, `scalar`, `timeseries`. Existing builds, queue, deployments,
logs and traces use table; metrics use timeseries. A new domain implements
`DataSource.ReadData(ctx, input)` with capability `data`, returning a DataResult
matching its declared renderer. No product branch or frontend script is needed.

Example card (inside a manifest):

```json
{
  "id": "inventory", "title": "Inventory", "capability": "data",
  "renderer": "table",
  "configSchema": {
    "type": "object", "additionalProperties": false,
    "properties": {"region": {"type": "string", "default": "local"}},
    "required": ["region"]
  }
}
```

ProviderContribution.ConfigSchema applies to non-secret Connection.Config.
CardContribution.ConfigSchema and QueryContribution.InputSchema describe query
input. Contract v1 supports closed flat objects with string/number/integer/boolean
fields, title/description, scalar enum/default, numeric minimum/maximum, string
maxLength and required. Unknown keywords, nested objects and additional properties
are rejected. Missing schemas accept empty objects. Defaults apply before client
construction/querying. Secrets use the existing credential field and OS store,
not Config. Schemas/inputs: at most 64 KiB, 40 fields, 100 choices per enum.

Common typed-source inputs: `targetId` for builds, `expression` and `period` in
seconds for metrics/logs/traces. Declare used inputs in the schema. Period defaults
to 3600, bounded to 1–86400 seconds. Generic data receives validated input unchanged.
Log/trace contributions fetch up to 100 rows.

Table columns declare string/number/boolean; cells are matching scalars or null.
Use strings for opaque IDs and integers beyond 2^53-1. Results are validated and
copied: at most 4 MiB, 64 columns, 2000 rows, 100 series and 20000 points. Series
timestamps are epoch seconds. Invalid results fail instead of being silently
truncated. Text is escaped; stale source/connection/plugin responses are rejected.
In-process cancellation remains cooperative.

Compiled definitions are still the package entry point. Trusted local ZIP install
and subprocess execution are phase 5; remaining provider migrations are phase 6.
This SDK is unreleased and may change during migration.

## Verification

Run `go test ./...` and `go vet ./...` from this directory. The application also
tests all five shipped providers by copying their production sources into an
external consumer module with only this SDK dependency and network resolution
disabled. Existing root integration tests continue to exercise the registry,
collector, HTTP fixtures and database.
