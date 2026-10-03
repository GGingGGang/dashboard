$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path $PSScriptRoot -Parent
Push-Location (Join-Path $taskRoot 'frontend')
try {
    npm ci
    if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
} finally { Pop-Location }
Push-Location $taskRoot
try {
    go run github.com/wailsapp/wails/v2/cmd/wails@v2.11.0 build -platform windows/amd64 -skipbindings -s
    if ($LASTEXITCODE -ne 0) { throw 'Windows build failed. Close the app before rebuilding.' }
} finally { Pop-Location }
