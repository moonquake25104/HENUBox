$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'project-env.ps1')
Push-Location (Join-Path $repoRoot 'App')
try {
    & node (Join-Path $PSScriptRoot 'generate-open-source-notices.mjs')
    if ($LASTEXITCODE -ne 0) { throw 'Notice generation failed' }
    & npx.cmd --no-install tsc -b
    if ($LASTEXITCODE -ne 0) { throw 'Typecheck failed' }
    & npx.cmd --no-install vite build --config vite.mobile.config.ts
    if ($LASTEXITCODE -ne 0) { throw 'Web build failed' }
} finally { Pop-Location }

