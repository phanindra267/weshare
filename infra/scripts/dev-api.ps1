# Starts the WeShare Go API in development mode (auto-migrate on boot).
#
# Usage:  powershell -File infra/scripts/dev-api.ps1

$ErrorActionPreference = "Stop"
$api = Join-Path (Split-Path -Parent $PSScriptRoot) "..\services\api"
$api = (Resolve-Path $api).Path

Push-Location $api
try {
  go run ./cmd/server
} finally {
  Pop-Location
}
