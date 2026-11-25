param(
  [string]$Url = "https://github.com/grainologic/papertrail",
  [string]$Tag = "latest",
  [string]$Dir = "",
  [switch]$DryRun,
  [switch]$Help
)

$ErrorActionPreference = "Stop"

function Show-Usage {
  Write-Output "Usage: install.ps1 [-Url URL] [-Tag TAG] [-Dir DIR] [-DryRun] [-Help]"
  Write-Output ""
  Write-Output "Downloads a prebuilt pt.exe release binary (no Go required) and places it at"
  Write-Output "<Dir>\pt.exe."
  Write-Output ""
  Write-Output "  -Url URL      base repository URL; default: https://github.com/grainologic/papertrail"
  Write-Output "  -Tag TAG      release tag; default: latest"
  Write-Output "  -Dir DIR      install directory; default: %LOCALAPPDATA%\papertrail\bin"
  Write-Output "  -DryRun       print the resolved plan without downloading"
  Write-Output "  -Help         show this help"
}

if ($Help) {
  Show-Usage
  exit 0
}

$Url = $Url.TrimEnd('/')
if (-not $Url) {
  $Url = "https://github.com/grainologic/papertrail"
}
if (-not $Tag) {
  $Tag = "latest"
}
if (-not $Dir) {
  $localAppData = if ($env:LOCALAPPDATA) { $env:LOCALAPPDATA } else { [Environment]::GetFolderPath("LocalApplicationData") }
  $Dir = Join-Path $localAppData "papertrail\bin"
}

$ri = [System.Runtime.InteropServices.RuntimeInformation]
if (-not $ri::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
  Write-Error "install.ps1 supports Windows only; use install.sh"
  exit 1
}

$arch = $ri::OSArchitecture.ToString()
switch ($arch) {
  "X64" { $arch = "amd64" }
  "Arm64" { $arch = "arm64" }
  default { Write-Error "unsupported architecture: $arch"; exit 1 }
}

$asset = "pt_windows_${arch}.zip"
if ($Tag -eq "latest") {
  $baseUrl = "$Url/releases/latest/download"
} else {
  $baseUrl = "$Url/releases/download/$Tag"
}
$downloadUrl = "$baseUrl/$asset"
$checksumUrl = "$baseUrl/checksums.txt"
$dest = Join-Path $Dir "pt.exe"

Write-Output "url:      $Url"
Write-Output "tag:      $Tag"
Write-Output "os/arch:  windows/$arch"
Write-Output "asset:    $asset"
Write-Output "download: $downloadUrl"
Write-Output "checksum: $checksumUrl"
Write-Output "dest:     $dest"

if ($DryRun) {
  exit 0
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  $assetPath = Join-Path $tmp $asset
  Invoke-WebRequest -Uri $downloadUrl -OutFile $assetPath
  $checksumPath = Join-Path $tmp "checksums.txt"
  Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath

  $expected = Get-Content $checksumPath |
    Where-Object { $_ -match "  $asset$" } |
    Select-Object -First 1 |
    ForEach-Object { ($_ -split " ")[0] }
  if (-not $expected) {
    Write-Error "no checksum entry for $asset"
    exit 1
  }
  $actual = (Get-FileHash $assetPath -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $expected.ToLowerInvariant()) {
    Write-Error "checksum mismatch for $asset"
    exit 1
  }

  New-Item -ItemType Directory -Path $Dir -Force | Out-Null
  $extractDir = Join-Path $tmp "extract"
  Expand-Archive -Path $assetPath -DestinationPath $extractDir -Force
  Copy-Item -Path (Join-Path $extractDir "pt.exe") -Destination $dest -Force
  Write-Output "installed $dest"

  $normalized = $Dir.TrimEnd('\')
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $userParts = @()
  if ($userPath) {
    $userParts = $userPath -split ';' | Where-Object { $_ -ne "" }
  }
  if (-not ($userParts | Where-Object { $_.TrimEnd('\') -ieq $normalized })) {
    $updated = if ($userParts.Count -gt 0) { ($userParts + $Dir) -join ';' } else { $Dir }
    [Environment]::SetEnvironmentVariable("Path", $updated, "User")
    Write-Output "added $Dir to user PATH (restart open shells to pick it up)"
  }
  $sessionParts = ($env:Path -split ';') | Where-Object { $_ -ne "" }
  if (-not ($sessionParts | Where-Object { $_.TrimEnd('\') -ieq $normalized })) {
    $env:Path = "$env:Path;$Dir"
  }
} finally {
  Remove-Item -Recurse -Force $tmp
}
