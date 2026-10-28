#!/usr/bin/env bash
set -euo pipefail
task_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
(
  cd "$task_root/sdk"
  go test ./... -count=1
  go vet ./...
)
cd "$task_root"
go test ./... -count=1
go vet ./...
