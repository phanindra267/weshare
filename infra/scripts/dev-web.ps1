# Starts the WeShare Next.js web app in development mode.
#
# Usage:  powershell -File infra/scripts/dev-web.ps1

$ErrorActionPreference = "Stop"
$web = Join-Path (Split-Path -Parent $PSScriptRoot) "..\apps\web"
$web = (Resolve-Path $web).Path

Push-Location $web
try {
  npm run dev
} finally {
  Pop-Location
}
