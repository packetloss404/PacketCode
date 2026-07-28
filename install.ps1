[CmdletBinding()]
param(
    [string]$Version = "latest",
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "Programs\PacketCode\bin")
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$repository = "packetloss404/packetcode"
$binary = "packetcode.exe"
$architecture = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()) {
    "x64" { "amd64" }
    "arm64" { "arm64" }
    default { throw "packetcode: unsupported Windows architecture: $_" }
}

if ($Version -eq "latest") {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repository/releases/latest"
    $Version = [string]$release.tag_name
}
if ([string]::IsNullOrWhiteSpace($Version)) {
    throw "packetcode: could not determine a release version"
}

$archive = "packetcode-windows-$architecture.zip"
$releaseBase = "https://github.com/$repository/releases/download/$Version"
$temporaryRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("packetcode-install-" + [guid]::NewGuid().ToString("N"))

try {
    New-Item -ItemType Directory -Path $temporaryRoot | Out-Null
    $archivePath = Join-Path $temporaryRoot $archive
    $checksumsPath = Join-Path $temporaryRoot "checksums.txt"

    Write-Host "Downloading packetcode $Version for windows/$architecture..."
    Invoke-WebRequest -Uri "$releaseBase/$archive" -OutFile $archivePath
    Invoke-WebRequest -Uri "$releaseBase/checksums.txt" -OutFile $checksumsPath

    $checksumLine = Get-Content -LiteralPath $checksumsPath |
        Where-Object { $_ -match "^\s*([0-9a-fA-F]{64})\s+\*?$([regex]::Escape($archive))\s*$" } |
        Select-Object -First 1
    if (-not $checksumLine) {
        throw "packetcode: checksum for $archive was not found in checksums.txt"
    }
    $expectedHash = ([regex]::Match($checksumLine, "^\s*([0-9a-fA-F]{64})")).Groups[1].Value.ToLowerInvariant()
    $actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "packetcode: checksum mismatch for $archive (expected $expectedHash, got $actualHash)"
    }

    $expanded = Join-Path $temporaryRoot "expanded"
    Expand-Archive -LiteralPath $archivePath -DestinationPath $expanded
    $source = Join-Path $expanded $binary
    if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
        throw "packetcode: $binary was not found in $archive"
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $destination = Join-Path $InstallDir $binary
    Copy-Item -LiteralPath $source -Destination $destination -Force

    $installedVersion = & $destination --version
    if ($LASTEXITCODE -ne 0) {
        throw "packetcode: installed binary failed its version probe"
    }
    Write-Host "$installedVersion"
    Write-Host "Installed to $destination"
    if (($env:PATH -split [System.IO.Path]::PathSeparator) -notcontains $InstallDir) {
        Write-Warning "$InstallDir is not on PATH. PacketADE can still detect this documented install location."
    }
}
finally {
    if (Test-Path -LiteralPath $temporaryRoot) {
        Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
    }
}
