# Runs database migrations against the local database.
#
# Usage:  powershell -File infra/scripts/migrate.ps1 up
#         powershell -File infra/scripts/migrate.ps1 down 1
#         powershell -File infra/scripts/migrate.ps1 status

param(
  [Parameter(Position = 0)]
  [ValidateSet("up", "down", "status")]
  [string]$Command = "up",

  [Parameter(Position = 1)]
  [int]$Steps = 0
)

$ErrorActionPreference = "Stop"
$api = Join-Path (Split-Path -Parent $PSScriptRoot) "..\services\api"
$api = (Resolve-Path $api).Path

Push-Location $api
try {
  if ($Command -eq "down" -and $Steps -gt 0) {
    go run ./cmd/migrate down $Steps
  } else {
    go run ./cmd/migrate $Command
  }
} finally {
  Pop-Location
}
