# Stops the local PostgreSQL container.
#
# Usage:  powershell -File infra/scripts/db-down.ps1
#         powershell -File infra/scripts/db-down.ps1 -Volumes   # also delete data

param(
  [switch]$Volumes
)

$ErrorActionPreference = "Stop"
$infra = Split-Path -Parent $PSScriptRoot

Push-Location $infra
try {
  if ($Volumes) {
    Write-Host "Stopping PostgreSQL and deleting the data volume..." -ForegroundColor Yellow
    docker compose down -v
  } else {
    Write-Host "Stopping PostgreSQL (data preserved)..." -ForegroundColor Cyan
    docker compose down
  }
} finally {
  Pop-Location
}
