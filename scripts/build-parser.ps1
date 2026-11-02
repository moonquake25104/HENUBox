$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'project-env.ps1')
if (-not $env:JAVA_HOME -or -not $env:ANDROID_HOME) { throw 'Set JAVA_HOME (JDK 21) and ANDROID_HOME (SDK) in this process first' }
if (-not $env:ANDROID_NDK_HOME) { $env:ANDROID_NDK_HOME = Join-Path $env:ANDROID_HOME 'ndk/27.1.12297006' }
if (-not (Test-Path $env:ANDROID_NDK_HOME)) { throw 'Install NDK 27.1.12297006 or set ANDROID_NDK_HOME' }
$env:PATH = (Join-Path $env:JAVA_HOME 'bin') + ';' + $env:PATH
$toolVersion = 'v0.0.0-20261011034528-197770444867'
$env:GOBIN = Join-Path $localRoot 'bin'
Push-Location (Join-Path $repoRoot 'App/server/mobile/bind')
try {
    foreach ($toolName in @('gomobile','gobind')) {
        if (-not (Test-Path (Join-Path $env:GOBIN ($toolName + '.exe')))) {
            & go install ('golang.org/x/mobile/cmd/' + $toolName + '@' + $toolVersion)
            if ($LASTEXITCODE -ne 0) { throw ('Cannot install ' + $toolName) }
        }
    }
    $aar = Join-Path $repoRoot 'App/mobile/android/henu-native/libs/henuparse.aar'
    New-Item -ItemType Directory -Path (Split-Path -Parent $aar) -Force | Out-Null
    & gomobile.exe bind -trimpath -target=android/arm64 -androidapi 28 -o $aar kairos/server/mobile/henuparse
    if ($LASTEXITCODE -ne 0) { throw 'Parser AAR build failed' }
} finally { Pop-Location }

