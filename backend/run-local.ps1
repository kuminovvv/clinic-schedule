$ErrorActionPreference = "Stop"
$envPath = Join-Path $PSScriptRoot ".env.local"
if (-not (Test-Path -LiteralPath $envPath)) {
    throw "Local database is not configured. Run .\backend\setup-local.ps1 first."
}

$databaseLine = Get-Content -LiteralPath $envPath | Where-Object { $_ -match '^DATABASE_URL=' } | Select-Object -First 1
if (-not $databaseLine) {
    throw "DATABASE_URL is missing from backend/.env.local."
}
$env:DATABASE_URL = $databaseLine.Substring("DATABASE_URL=".Length)
$listenLine = Get-Content -LiteralPath $envPath | Where-Object { $_ -match '^LISTEN_ADDR=' } | Select-Object -First 1
$env:LISTEN_ADDR = if ($listenLine) { $listenLine.Substring("LISTEN_ADDR=".Length) } else { "127.0.0.1:8080" }
$env:CLINIC_TIMEZONE = "Asia/Omsk"
$env:TZ = "Asia/Omsk"

Push-Location $PSScriptRoot
try {
    go run ./cmd/api
} finally {
    Pop-Location
    Remove-Item Env:DATABASE_URL -ErrorAction SilentlyContinue
    Remove-Item Env:LISTEN_ADDR -ErrorAction SilentlyContinue
    Remove-Item Env:CLINIC_TIMEZONE -ErrorAction SilentlyContinue
    Remove-Item Env:TZ -ErrorAction SilentlyContinue
}
