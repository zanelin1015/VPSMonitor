Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RootDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$DistDir = Join-Path $RootDir "dist"
$CacheDir = Join-Path $RootDir ".cache/go-build"
$VersionFile = Join-Path $RootDir "VERSION"
$VersionPackage = "bridge-core/internal/version"

function Test-Semver([string]$Version) {
  return $Version -match '^[0-9]+\.[0-9]+\.[0-9]+$'
}

function Get-NextPatchVersion([string]$Version) {
  if (-not (Test-Semver $Version)) {
    throw "invalid semantic version: $Version"
  }
  $parts = $Version.Split('.')
  $patch = [int]$parts[2] + 1
  return "$($parts[0]).$($parts[1]).$patch"
}

function Resolve-BuildVersion() {
  $requested = $env:VPSMONITOR_BUILD_VERSION
  if ($requested) {
    if (-not (Test-Semver $requested)) {
      throw "VPSMONITOR_BUILD_VERSION must use MAJOR.MINOR.PATCH, got: $requested"
    }
    Set-Content -Path $VersionFile -Value $requested -NoNewline
    Add-Content -Path $VersionFile -Value ""
    return $requested
  }

  if (-not (Test-Path $VersionFile)) {
    Set-Content -Path $VersionFile -Value "0.1.0"
    return "0.1.0"
  }

  $current = ((Get-Content -Path $VersionFile -Raw).Trim())
  $next = Get-NextPatchVersion $current
  Set-Content -Path $VersionFile -Value $next
  return $next
}

function Package-Component([string]$AppName, [string]$Entrypoint, [string]$GoOs, [string]$TargetArch, [string]$BuildVersion, [string]$BuildTime, [string]$GitCommit) {
  $GoArch = $TargetArch
  $GoArm = ""
  if ($TargetArch -eq "arm" -or $TargetArch -eq "armv7") {
    $GoArch = "arm"
    $GoArm = "7"
    $TargetArch = "arm"
  }
  $ext = ""
  if ($GoOs -eq "windows") {
    $ext = ".exe"
  }

  $packagePrefix = if ($env:PACKAGE_PREFIX) { $env:PACKAGE_PREFIX } else { "VPSMonitor" }
  $packageRole = $AppName -replace "^bridge-", ""
  $packageName = "$packagePrefix-$packageRole-$GoOs-$TargetArch"
  $outputDir = Join-Path $DistDir $packageName
  $configName = "$packageRole.json"
  $configExample = Join-Path $RootDir "config\$packageRole.example.json"
  $binaryPath = Join-Path $outputDir "$AppName$ext"

  if (Test-Path $outputDir) {
    Remove-Item -Recurse -Force $outputDir
  }
  Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $DistDir "$packageName.zip")
  Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $DistDir "$packageName.tar.gz")
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $DistDir "$AppName-$GoOs-$GoArch")
  Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $DistDir "$AppName-$GoOs-$GoArch.zip")
  Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $DistDir "$AppName-$GoOs-$GoArch.tar.gz")
  New-Item -ItemType Directory -Force -Path (Join-Path $outputDir "config") | Out-Null

  $env:GOOS = $GoOs
  $env:GOARCH = $GoArch
  $env:GOARM = $GoArm
  $ldflags = "-s -w -X $VersionPackage.Version=$BuildVersion -X $VersionPackage.BuildTime=$BuildTime -X $VersionPackage.GitCommit=$GitCommit"
  go build -trimpath -ldflags=$ldflags -o $binaryPath $Entrypoint

  Copy-Item $configExample (Join-Path $outputDir "config\$configName")
  Copy-Item (Join-Path $RootDir "README.md") (Join-Path $outputDir "README.md")
  Copy-Item $VersionFile (Join-Path $outputDir "VERSION")

  if ($GoOs -eq "windows") {
    Copy-Item (Join-Path $RootDir "scripts\templates\run-$AppName.bat") (Join-Path $outputDir "run.bat")
    if ($AppName -eq "bridge-client") {
      Copy-Item (Join-Path $RootDir "install.ps1") (Join-Path $outputDir "install.ps1")
      Copy-Item (Join-Path $RootDir "install-client.cmd") (Join-Path $outputDir "install-client.cmd")
    }
    Compress-Archive -Path $outputDir -DestinationPath (Join-Path $DistDir "$packageName.zip") -Force
  }
  else {
    Copy-Item (Join-Path $RootDir "scripts\templates\run-$AppName.sh") (Join-Path $outputDir "run.sh")
    Copy-Item (Join-Path $RootDir "install.sh") (Join-Path $outputDir "install.sh")
    if ($AppName -eq "bridge-client") {
      Copy-Item (Join-Path $RootDir "install-openwrt.sh") (Join-Path $outputDir "install-openwrt.sh")
    }
    if (Get-Command tar -ErrorAction SilentlyContinue) {
      & tar -czf (Join-Path $DistDir "$packageName.tar.gz") -C $DistDir $packageName
    }
  }
}

if (-not $env:GOCACHE) {
  $env:GOCACHE = $CacheDir
}
$env:CGO_ENABLED = "0"

New-Item -ItemType Directory -Force -Path $DistDir | Out-Null
New-Item -ItemType Directory -Force -Path $CacheDir | Out-Null

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
  throw "npm is required to build the embedded web console."
}

$BuildVersion = Resolve-BuildVersion
$BuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
try {
  $GitCommit = (& git -C $RootDir rev-parse --short HEAD 2>$null).Trim()
}
catch {
  $GitCommit = ""
}

Push-Location $RootDir
try {
  if (-not (Test-Path (Join-Path $RootDir "web\node_modules"))) {
    Push-Location (Join-Path $RootDir "web")
    try {
      npm install
    }
    finally {
      Pop-Location
    }
  }

  Push-Location (Join-Path $RootDir "web")
  try {
    npm run build
  }
  finally {
    Pop-Location
  }

  go test ./...

  $targets = @("linux/amd64", "linux/arm64", "linux/arm", "windows/amd64", "windows/arm64")

  foreach ($target in $targets) {
    $parts = $target.Split("/")
    $goos = $parts[0]
    $goarch = $parts[1]

    Package-Component "bridge-server" "./cmd/bridge-server" $goos $goarch $BuildVersion $BuildTime $GitCommit
    Package-Component "bridge-client" "./cmd/bridge-client" $goos $goarch $BuildVersion $BuildTime $GitCommit
  }

  Write-Host "version $BuildVersion"
  Write-Host "build artifacts written to $DistDir"
}
finally {
  Pop-Location
}
