Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RootDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$DistDir = Join-Path $RootDir "dist"
$CacheDir = Join-Path $RootDir ".cache/go-build"

if (-not $env:GOCACHE) {
  $env:GOCACHE = $CacheDir
}
$env:CGO_ENABLED = "0"

New-Item -ItemType Directory -Force -Path $DistDir | Out-Null
New-Item -ItemType Directory -Force -Path $CacheDir | Out-Null

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
  throw "npm is required to build the embedded web console."
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

  $targets = @("linux/amd64", "windows/amd64")

  foreach ($target in $targets) {
    $parts = $target.Split("/")
    $goos = $parts[0]
    $goarch = $parts[1]

    Package-Component "bridge-server" "./cmd/bridge-server" $goos $goarch
    Package-Component "bridge-client" "./cmd/bridge-client" $goos $goarch
  }
}
finally {
  Pop-Location
}

function Package-Component([string]$AppName, [string]$Entrypoint, [string]$GoOs, [string]$GoArch) {
  $ext = ""
  if ($GoOs -eq "windows") {
    $ext = ".exe"
  }

  $packagePrefix = if ($env:PACKAGE_PREFIX) { $env:PACKAGE_PREFIX } else { "VPSMonitor" }
  $packageRole = $AppName -replace "^bridge-", ""
  $packageName = "$packagePrefix-$packageRole-$GoOs-$GoArch"
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
  go build -trimpath -ldflags="-s -w" -o $binaryPath $Entrypoint

  Copy-Item $configExample (Join-Path $outputDir "config\$configName")
  Copy-Item (Join-Path $RootDir "README.md") (Join-Path $outputDir "README.md")

  if ($GoOs -eq "windows") {
    Copy-Item (Join-Path $RootDir "scripts\templates\run-$AppName.bat") (Join-Path $outputDir "run.bat")
    Compress-Archive -Path $outputDir -DestinationPath (Join-Path $DistDir "$packageName.zip") -Force
  }
  else {
    Copy-Item (Join-Path $RootDir "scripts\templates\run-$AppName.sh") (Join-Path $outputDir "run.sh")
    if ($AppName -eq "bridge-server") {
      Copy-Item (Join-Path $RootDir "install.sh") (Join-Path $outputDir "install.sh")
    }
    if (Get-Command tar -ErrorAction SilentlyContinue) {
      & tar -czf (Join-Path $DistDir "$packageName.tar.gz") -C $DistDir $packageName
    }
  }
}
