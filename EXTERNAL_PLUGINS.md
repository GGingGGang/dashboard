# External packages (contract v1)

Open **플러그인**, confirm that you trust the package's native code, then choose
**신뢰한 ZIP 설치 · 업그레이드**. Restart the app to load it. Removing a package
also applies after restart. Enable/disable applies immediately to loaded plugins.
Connections, credentials, archives and personal cards are preserved.

Packages run with the user's permissions. Process separation is a lifecycle and
fault boundary, not an OS sandbox or a signature guarantee. Install only trusted
code. The host does not pass secrets in argv, environment variables or logs;
configuration and that connection's credential are sent over the private stdin
pipe. Provider code itself must not log secrets. Stdout is reserved for RPC.

## Build the working example

From Git Bash at the repository root:

```bash
bash scripts/package-example.sh
```

This builds the independent SDK-only example and creates
`build/bin/plugin-example/example-inventory.zip`. Installing this ZIP needs no
core rebuild. After restart, add an Inventory example connection (the example
requires no real server; use `https://example.invalid` for its saved endpoint),
then open its dashboard. The data is synthetic, not an inventory integration.

## ZIP layout and validation

`manifest.json` is at the archive root. Each provider declaration includes
`info` (ProviderInfo) and an in-package `executable` such as `bin/provider.exe`.
See [the example manifest](examples/external-provider/manifest.json). UI-only
plugins may omit providers. Capabilities must match the running executable.
The package can contain other regular files needed by the executable.

Limits: 128 MiB compressed and total expanded data, 64 MiB per file, 1024 entries,
256 KiB manifest, 8 MiB RPC frames. Absolute/traversal/Windows device paths,
alternate streams, duplicate case-insensitive paths and symlinks are rejected.
Extraction happens in a new staging directory. Manifest/provider/schema checks
complete before switching the next-start index. Built-in IDs cannot be replaced.

Packages live under `%LOCALAPPDATA%/IDPDashboard/plugins` (or its isolated demo
subdirectory). An atomically replaced `index.json` selects content-addressed
package directories. Active processes keep using their original files until
shutdown. Old package directories are retained; automatic garbage collection and
signature verification are not implemented. An invalid index is preserved and
reported. An invalid package is isolated from other plugins at startup.

## Provider process protocol

The independent `sdk/protocol` package implements newline-delimited JSON-RPC 2.0
and supports simultaneous requests in both directions. Call
`protocol.Serve(os.Stdin, os.Stdout, definitions...)` from your executable.
One process serves one configured connection; the host starts it lazily.

`initialize` negotiates protocol version 1, provider kind, actual capabilities and
optional discovery. `check`, `discover`, `publish`, `builds`, `build`, `queue`,
`deployments`, `query`, `logs`, `traces`, `trace`, `data` and `release` use opaque
source handles. `source.register`, `source.update`, and `source.delete` call the
connection-scoped host registrar in reverse. The host owns accepted handles and
releases them only after local leases finish. SDK server resources also retain
active calls before closing. No upstream mutation is implied by these methods.

Request IDs correlate replies. `$/cancelRequest` propagates cancellation. The
host terminates a process on a cancelled/timed-out call or invalid/disconnected
transport; the next source use rebuilds the scope. Failed initialization has a
short retry delay. Closing the final source closes the process. The host launches
without a console window on Windows. Subprocess descendants are not sandboxed.

Upstream API status and partial typed results are preserved in JSON-RPC error
`data`; oversized/malformed frames close the transport. Only declared capability
interfaces are resolved by core consumers. The extension still depends only on
the SDK; it never imports registry, storage, credentials or application code.

The SDK also exposes `JSONSource` (`json` RPC method/capability) for the legacy
HTTP JSON/jq contract. Generic JSON table/scalar contributions use `data`.
All six built-ins publish scoped sources; external providers without an explicit
CollectionSource use `default` for collection and legacy query APIs.
