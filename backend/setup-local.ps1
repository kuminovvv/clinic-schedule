$ErrorActionPreference = "Stop"

$psql = "D:\Program Files\PostgreSQL\18\bin\psql.exe"
if (-not (Test-Path -LiteralPath $psql)) {
    $command = Get-Command psql -ErrorAction SilentlyContinue
    if (-not $command) {
        throw "psql.exe not found. Install PostgreSQL client tools or update the psql path in this script."
    }
    $psql = $command.Source
}

$envPath = Join-Path $PSScriptRoot ".env.local"
if (Test-Path -LiteralPath $envPath) {
    throw "backend/.env.local already exists. Keeping the existing local credentials unchanged."
}

$securePassword = Read-Host "PostgreSQL administrator password for role 'postgres' (input hidden)" -AsSecureString
$passwordPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)
$adminPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPointer)
$random = [System.Security.Cryptography.RandomNumberGenerator]::Create()
$secretBytes = New-Object byte[] 32
$random.GetBytes($secretBytes)
$appPassword = ([BitConverter]::ToString($secretBytes)).Replace("-", "").ToLowerInvariant()
$databaseCreated = $false
$roleCreated = $false

function Invoke-PostgresQuery([string]$database, [string]$query) {
    $output = & $psql -X -w -h 127.0.0.1 -U postgres -d $database -v ON_ERROR_STOP=1 -Atc $query
    if ($LASTEXITCODE -ne 0) {
        throw "PostgreSQL command failed. Check the administrator password and server status."
    }
    return ($output | Out-String).Trim()
}

try {
    $env:PGPASSWORD = $adminPassword
    if ((Invoke-PostgresQuery "postgres" "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='clinic_schedule_app')") -eq "t") {
        throw "Role clinic_schedule_app already exists. No database changes were made."
    }
    if ((Invoke-PostgresQuery "postgres" "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname='clinic_schedule')") -eq "t") {
        throw "Database clinic_schedule already exists. No database changes were made."
    }

    @"
CREATE ROLE clinic_schedule_app LOGIN PASSWORD '$appPassword';
"@ | & $psql -X -w -h 127.0.0.1 -U postgres -d postgres -v ON_ERROR_STOP=1 -f -
    if ($LASTEXITCODE -ne 0) { throw "Could not create the application database role." }
    $roleCreated = $true

    @"
CREATE DATABASE clinic_schedule OWNER clinic_schedule_app;
"@ | & $psql -X -w -h 127.0.0.1 -U postgres -d postgres -v ON_ERROR_STOP=1 -f -
    if ($LASTEXITCODE -ne 0) { throw "Could not create the application database." }
    $databaseCreated = $true

    $databaseUrl = "postgres://clinic_schedule_app:$appPassword@127.0.0.1:5432/clinic_schedule?sslmode=disable"
    Set-Content -LiteralPath $envPath -Value @(
        "DATABASE_URL=$databaseUrl",
        "CLINIC_TIMEZONE=Asia/Omsk",
        "LISTEN_ADDR=127.0.0.1:8081"
    ) -Encoding ascii
    Write-Host "Created the local clinic_schedule database and backend/.env.local."
    Write-Host "The Go API will apply migrations when started."
} catch {
    if ($roleCreated -and -not $databaseCreated) {
        try { Invoke-PostgresQuery "postgres" "DROP ROLE IF EXISTS clinic_schedule_app" | Out-Null } catch { }
    }
    throw
} finally {
    Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPointer)
    $random.Dispose()
    [Array]::Clear($secretBytes, 0, $secretBytes.Length)
    $adminPassword = $null
    $appPassword = $null
}
