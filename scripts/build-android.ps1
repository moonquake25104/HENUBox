param([switch]$InstallDependencies)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'project-env.ps1')
if (-not $env:JAVA_HOME -or -not $env:ANDROID_HOME) { throw 'Set JAVA_HOME (JDK 21) and ANDROID_HOME (SDK) in this process first' }
$env:PATH = (Join-Path $env:JAVA_HOME 'bin') + ';' + $env:PATH
if ($InstallDependencies) {
    foreach ($relative in @('App','App/mobile')) {
        Push-Location (Join-Path $repoRoot $relative)
        try { & npm.cmd ci; if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' } } finally { Pop-Location }
    }
}
$aar = Join-Path $repoRoot 'App/mobile/android/henu-native/libs/henuparse.aar'
if (-not (Test-Path $aar)) { & (Join-Path $PSScriptRoot 'build-parser.ps1') }
& (Join-Path $PSScriptRoot 'build-mobile-web.ps1')
$debugKey = Join-Path $localRoot 'signing/debug.keystore'
if (-not (Test-Path $debugKey)) {
    & keytool.exe -genkeypair -keystore $debugKey -storepass android -keypass android -alias androiddebugkey -keyalg RSA -keysize 2048 -validity 10000 -dname 'CN=Android Debug,O=Android,C=US' -noprompt
    if ($LASTEXITCODE -ne 0) { throw 'Debug signing key generation failed' }
}
Push-Location (Join-Path $repoRoot 'App/mobile')
try { & npx.cmd --no-install cap sync android; if ($LASTEXITCODE -ne 0) { throw 'Capacitor sync failed' } } finally { Pop-Location }
Push-Location (Join-Path $repoRoot 'App/mobile/android')
try {
    & ./gradlew.bat --no-daemon :app:assembleDebug -PhenuboxShrink
    if ($LASTEXITCODE -ne 0) { throw 'Android build failed' }
} finally { Pop-Location }
Write-Output (Join-Path $repoRoot 'App/mobile/android/app/build/outputs/apk/debug/app-debug.apk')

