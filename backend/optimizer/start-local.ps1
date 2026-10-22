# Local-only launcher. Loads developer credentials from the ignored .env.local.
# Do not copy this process configuration to the production VPS.
$ErrorActionPreference = 'Stop'
$backend = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$envFile = Join-Path $backend '.env.local'
if (-not (Test-Path -LiteralPath $envFile)) {
    throw "Missing local backend configuration: $envFile"
}
foreach ($line in Get-Content -LiteralPath $envFile) {
    if ($line -notmatch '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$') {
        continue
    }
    $name = $Matches[1]
    $value = $Matches[2]
    if ($value.Length -ge 2 -and (($value[0] -eq '"' -and $value[-1] -eq '"') -or ($value[0] -eq "'" -and $value[-1] -eq "'"))) {
        $value = $value.Substring(1, $value.Length - 2)
    }
    [Environment]::SetEnvironmentVariable($name, $value, 'Process')
}
$env:CLINIC_COURSE_OPTIMIZER = 'cp-sat'
$python = Join-Path $backend '.venv-cpsat\Scripts\python.exe'
if (-not (Test-Path -LiteralPath $python)) {
    throw "Missing CP-SAT Python environment: $python"
}
$env:CLINIC_OPTIMIZER_PYTHON = $python
$env:CLINIC_OPTIMIZER_SCRIPT = (Join-Path $backend 'optimizer\cpsat_course.py')
Set-Location $backend
& 'C:\Program Files\Go\bin\go.exe' run ./cmd/api
exit $LASTEXITCODE
