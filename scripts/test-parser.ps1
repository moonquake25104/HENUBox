$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'project-env.ps1')
Push-Location (Join-Path $repoRoot 'App/server')
try {
    & go test ./internal/domain/henu ./internal/domain/classroom ./mobile/henuparse
    if ($LASTEXITCODE -ne 0) { throw 'Parser tests failed' }
} finally { Pop-Location }

