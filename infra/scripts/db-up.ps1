# Starts the local PostgreSQL container and waits until it reports healthy.
#
# Usage:  powershell -ExecutionPolicy Bypass -File infra/scripts/db-up.ps1
#         powershell ... -File infra/scripts/db-up.ps1 -Recreate   # wipes data

param(
  [switch]$Recreate
)

$ErrorActionPreference = "Stop"
$infra = Split-Path -Parent $PSScriptRoot

Push-Location $infra
try {
  if ($Recreate) {
    Write-Host "Recreating database volume (all local data will be lost)..." -ForegroundColor Yellow
    docker compose down -v
  }

  Write-Host "Starting PostgreSQL..." -ForegroundColor Cyan
  docker compose up -d

  $deadline = (Get-Date).AddSeconds(60)
  while ((Get-Date) -lt $deadline) {
    $status = $null
    try {
      $status = docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' weshare-postgres 2>$null
    } catch {
      $status = $null
    }
    if ($status -eq "healthy") {
      Write-Host "PostgreSQL is healthy." -ForegroundColor Green
      exit 0
    }
    Start-Sleep -Seconds 2
  }

  Write-Error "Timed out waiting for PostgreSQL to become healthy."
} finally {
  Pop-Location
}
