# Process-local configuration. No registry or global PATH changes.
$repoRoot = Split-Path -Parent $PSScriptRoot
$localRoot = Join-Path $repoRoot '.local'
$cacheRoot = if ($env:HENUBOX_CACHE_ROOT) { [IO.Path]::GetFullPath($env:HENUBOX_CACHE_ROOT) } else { Join-Path $localRoot 'cache' }
foreach ($dir in @('tmp','cache/npm','cache/node','cache/go-build','cache/go-mod','cache/gradle','config/android','config/appdata','cache/localappdata','user','go','bin','signing')) {
    New-Item -ItemType Directory -Path (Join-Path $localRoot $dir) -Force | Out-Null
}
$env:TEMP = Join-Path $localRoot 'tmp'; $env:TMP = $env:TEMP
$env:HOME = Join-Path $localRoot 'user'; $env:USERPROFILE = $env:HOME
$env:APPDATA = Join-Path $localRoot 'config/appdata'
$env:LOCALAPPDATA = Join-Path $localRoot 'cache/localappdata'
$env:npm_config_cache = Join-Path $cacheRoot 'npm'
$env:npm_config_userconfig = Join-Path $localRoot 'config/npm-user.npmrc'
$env:npm_config_globalconfig = Join-Path $localRoot 'config/npm-global.npmrc'
$env:npm_config_update_notifier = 'false'
$env:NODE_COMPILE_CACHE = Join-Path $cacheRoot 'node'
$env:PYTHONDONTWRITEBYTECODE = '1'
$env:GOPATH = Join-Path $localRoot 'go'
$env:GOCACHE = Join-Path $cacheRoot 'go-build'
$env:GOMODCACHE = Join-Path $cacheRoot 'go-mod'
$env:GOENV = Join-Path $localRoot 'config/go-env'
$env:GOTMPDIR = $env:TEMP; $env:GOTOOLCHAIN = 'local'
$env:GOWORK = 'off'
$env:GRADLE_USER_HOME = Join-Path $cacheRoot 'gradle'
$env:ANDROID_USER_HOME = Join-Path $localRoot 'config/android'
Remove-Item Env:ANDROID_SDK_HOME -ErrorAction SilentlyContinue
$env:JAVA_TOOL_OPTIONS = '-Duser.home=' + $env:HOME.Replace('\','/') + ' -Djava.io.tmpdir=' + $env:TEMP.Replace('\','/')
$env:PATH = (Join-Path $localRoot 'bin') + ';' + $env:PATH

