param(
  [ValidateSet("client")]
  [string]$Target = "client"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$Repo = if ($env:VPSMONITOR_REPO) { $env:VPSMONITOR_REPO } else { "zanelin1015/VPSMonitor" }
$Version = if ($env:VPSMONITOR_VERSION) { $env:VPSMONITOR_VERSION } else { "latest" }
$PackagePrefix = if ($env:VPSMONITOR_PACKAGE_PREFIX) { $env:VPSMONITOR_PACKAGE_PREFIX } else { "VPSMonitor" }
$InstallDir = if ($env:VPSMONITOR_CLIENT_DIR) { $env:VPSMONITOR_CLIENT_DIR } else { Join-Path $env:ProgramFiles "VPSMonitor\client" }
$ServiceName = if ($env:VPSMONITOR_CLIENT_SERVICE) { $env:VPSMONITOR_CLIENT_SERVICE } else { "VPSMonitorClient" }
$AssumeYesRaw = if ($env:VPSMONITOR_ASSUME_YES) { $env:VPSMONITOR_ASSUME_YES } elseif ($env:VPSMONITOR_NON_INTERACTIVE) { $env:VPSMONITOR_NON_INTERACTIVE } else { "" }
$ForceConfigRaw = if ($env:VPSMONITOR_FORCE_CONFIG) { $env:VPSMONITOR_FORCE_CONFIG } else { "" }
$AssumeYes = @("1", "true", "yes", "y", "on") -contains $AssumeYesRaw.ToLowerInvariant()
$ForceConfig = @("1", "true", "yes", "y", "on") -contains $ForceConfigRaw.ToLowerInvariant()

function Write-Info([string]$Message) { Write-Host $Message -ForegroundColor Cyan }
function Write-Ok([string]$Message) { Write-Host $Message -ForegroundColor Green }
function Write-Warn([string]$Message) { Write-Host $Message -ForegroundColor Yellow }

function Assert-Admin {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = [Security.Principal.WindowsPrincipal]::new($identity)
  if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Please run PowerShell or CMD as Administrator."
  }
}

function Get-Arch {
  switch ($env:PROCESSOR_ARCHITECTURE.ToLowerInvariant()) {
    "amd64" { "amd64"; break }
    "arm64" { "arm64"; break }
    default { throw "Unsupported CPU architecture: $env:PROCESSOR_ARCHITECTURE" }
  }
}

function Read-Default([string]$Label, [string]$Default) {
  if ($AssumeYes) { return $Default }
  $value = Read-Host "$Label [$Default]"
  if ([string]::IsNullOrWhiteSpace($value)) { return $Default }
  return $value.Trim()
}

function Read-Required([string]$Label, [string]$Default = "") {
  if ($AssumeYes) {
    if ([string]::IsNullOrWhiteSpace($Default)) { throw "$Label is required for non-interactive installation." }
    return $Default
  }
  while ($true) {
    $value = Read-Host $Label
    if (-not [string]::IsNullOrWhiteSpace($value)) { return $value.Trim() }
    Write-Warn "This value is required."
  }
}

function Confirm-No([string]$Label) {
  if ($AssumeYes) { return $false }
  $value = (Read-Host "$Label [y/N]").ToLowerInvariant()
  return $value -eq "y" -or $value -eq "yes"
}

function Get-PackageUrl([string]$Arch) {
  if ($env:VPSMONITOR_CLIENT_PACKAGE_URL) { return $env:VPSMONITOR_CLIENT_PACKAGE_URL }
  if ($env:VPSMONITOR_PACKAGE_URL) { return $env:VPSMONITOR_PACKAGE_URL }
  $packageName = "$PackagePrefix-client-windows-$Arch.zip"
  if ($env:VPSMONITOR_BASE_URL) { return "$($env:VPSMONITOR_BASE_URL.TrimEnd('/'))/$packageName" }
  if ($Version -eq "latest") { return "https://github.com/$Repo/releases/latest/download/$packageName" }
  return "https://github.com/$Repo/releases/download/$Version/$packageName"
}

function Write-ClientConfig([string]$Path, [string]$ServerUrl, [string]$RegistrationToken, [bool]$SkipTlsVerify, [string]$PollInterval, [int]$RequestTimeoutSeconds) {
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Path) | Out-Null
  $payload = [ordered]@{
    registration_token = $RegistrationToken
    agent_token = ""
    server_url = $ServerUrl
    server_skip_tls_verify = $SkipTlsVerify
    poll_interval = $PollInterval
    request_timeout_seconds = $RequestTimeoutSeconds
  }
  $payload | ConvertTo-Json -Depth 5 | Set-Content -Path $Path -Encoding UTF8
}

function Install-ClientService([string]$BinaryPath, [string]$ConfigPath) {
  $existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
  if ($existing) {
    if ($existing.Status -ne "Stopped") {
      Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
      Start-Sleep -Seconds 2
    }
    & sc.exe delete $ServiceName | Out-Null
    Start-Sleep -Seconds 2
  }
  $binPath = '"{0}" -config "{1}"' -f $BinaryPath, $ConfigPath
  & sc.exe create $ServiceName binPath= $binPath start= auto DisplayName= "VPSMonitor Client" | Out-Null
  & sc.exe description $ServiceName "VPSMonitor Windows Client" | Out-Null
  & sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/5000/restart/5000 | Out-Null
  Start-Service -Name $ServiceName
}

Assert-Admin
$Arch = Get-Arch
$InstallDir = Read-Default "Install directory for VPSMonitor client" $InstallDir
$ConfigPath = Join-Path $InstallDir "config\client.json"
$BinaryPath = Join-Path $InstallDir "bridge-client.exe"

Write-Info "VPSMonitor Windows client installer"
Write-Host "  Arch: windows/$Arch"
Write-Host "  Install: $InstallDir"
Write-Host "  Service: $ServiceName"

$ServerUrl = if ($env:VPSMONITOR_SERVER_URL) { $env:VPSMONITOR_SERVER_URL } else { "http://SERVER_IP:8090" }
$RegistrationToken = if ($env:VPSMONITOR_REGISTRATION_TOKEN) { $env:VPSMONITOR_REGISTRATION_TOKEN } else { "" }
$SkipTlsVerifyRaw = if ($env:VPSMONITOR_SERVER_SKIP_TLS_VERIFY) { $env:VPSMONITOR_SERVER_SKIP_TLS_VERIFY } else { "false" }
$PollInterval = if ($env:VPSMONITOR_POLL_INTERVAL) { $env:VPSMONITOR_POLL_INTERVAL } else { "30s" }
$RequestTimeout = if ($env:VPSMONITOR_REQUEST_TIMEOUT_SECONDS) { [int]$env:VPSMONITOR_REQUEST_TIMEOUT_SECONDS } else { 15 }

if ((Test-Path $ConfigPath) -and -not $ForceConfig) {
  Write-Warn "Existing client config found: $ConfigPath"
  if (Confirm-No "Overwrite and reconfigure it") { $ForceConfig = $true } else { Write-Info "Keeping existing client config." }
}

if (-not (Test-Path $ConfigPath) -or $ForceConfig) {
  $ServerUrl = Read-Default "Server URL" $ServerUrl
  if ([string]::IsNullOrWhiteSpace($RegistrationToken)) { $RegistrationToken = Read-Required "Client registration token" }
  $SkipTlsVerifyRaw = Read-Default "Skip server TLS verification? true/false" $SkipTlsVerifyRaw
  $PollInterval = Read-Default "Poll interval" $PollInterval
  $RequestTimeout = [int](Read-Default "Request timeout seconds" ([string]$RequestTimeout))
  $SkipTlsVerify = @("1", "true", "yes", "y", "on") -contains $SkipTlsVerifyRaw.ToLowerInvariant()
  Write-ClientConfig $ConfigPath $ServerUrl $RegistrationToken $SkipTlsVerify $PollInterval $RequestTimeout
}

$TempDir = Join-Path $env:TEMP ("vpsmonitor-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $TempDir | Out-Null
try {
  $PackageUrl = Get-PackageUrl $Arch
  $ZipPath = Join-Path $TempDir "client.zip"
  Write-Info "Downloading $PackageUrl"
  Invoke-WebRequest -Uri $PackageUrl -OutFile $ZipPath -UseBasicParsing
  Expand-Archive -Path $ZipPath -DestinationPath $TempDir -Force
  $DownloadedBinary = Get-ChildItem -Path $TempDir -Filter "bridge-client.exe" -Recurse | Select-Object -First 1
  if (-not $DownloadedBinary) { throw "bridge-client.exe was not found in package." }
  New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
  Copy-Item $DownloadedBinary.FullName $BinaryPath -Force
  $Readme = Get-ChildItem -Path $TempDir -Filter "README.md" -Recurse | Select-Object -First 1
  if ($Readme) { Copy-Item $Readme.FullName (Join-Path $InstallDir "README.md") -Force }
}
finally {
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $TempDir
}

Install-ClientService $BinaryPath $ConfigPath
Write-Ok "VPSMonitor client installed."
Write-Host "  Service: $ServiceName"
Write-Host "  Config:  $ConfigPath"
Write-Host "  Status:  Get-Service $ServiceName"
Write-Host "  Logs:    Event Viewer or sc.exe query $ServiceName"
