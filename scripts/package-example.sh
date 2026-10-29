#!/usr/bin/env bash
set -euo pipefail
task_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$task_root/build/bin/plugin-example"
(cd "$task_root/examples/external-provider" && go build -o ../../build/bin/plugin-example/provider.exe .)
cd "$task_root"
python - <<'PY'
from zipfile import ZipFile, ZIP_DEFLATED
from pathlib import Path
output = Path('build/bin/plugin-example/example-inventory.zip')
with ZipFile(output, 'w', ZIP_DEFLATED) as package:
    package.write('examples/external-provider/manifest.json', 'manifest.json')
    package.write('build/bin/plugin-example/provider.exe', 'provider.exe')
print(output.resolve())
PY
