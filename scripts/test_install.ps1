$ErrorActionPreference = 'Stop'

$architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
if ($architecture -notin @('X64', 'Arm64')) {
    throw "Unsupported test architecture: $architecture"
}
$arch = if ($architecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$archiveName = "vexlo-windows-$arch.zip"
$binaryName = "vexlo-windows-$arch.exe"
$tempBase = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
$testRoot = Join-Path $tempBase ("vexlo-installer-test-" + [guid]::NewGuid().ToString('N'))
$fixture = Join-Path $testRoot 'release'
$originalUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$originalProcessPath = $env:Path
$originalLocalAppData = $env:LOCALAPPDATA
New-Item -ItemType Directory -Path $fixture | Out-Null

function Invoke-WebRequest {
    param([string]$Uri, [string]$OutFile)
    Copy-Item -LiteralPath (Join-Path $fixture ([System.IO.Path]::GetFileName($Uri))) -Destination $OutFile
}

try {
    $binaryPath = Join-Path $fixture $binaryName
    [System.IO.File]::WriteAllText($binaryPath, 'fixture cli')
    Compress-Archive -LiteralPath $binaryPath -DestinationPath (Join-Path $fixture $archiveName)
    $checksum = (Get-FileHash -LiteralPath (Join-Path $fixture $archiveName) -Algorithm SHA256).Hash.ToLowerInvariant()
    [System.IO.File]::WriteAllText((Join-Path $fixture 'SHA256SUMS.txt'), "$checksum  $archiveName`n")
    $env:LOCALAPPDATA = Join-Path $testRoot 'appdata'

    & "$PSScriptRoot/install.ps1"
    $installed = Join-Path $env:LOCALAPPDATA 'Programs\Vexlo\vexlo.exe'
    if ([System.IO.File]::ReadAllText($installed) -ne 'fixture cli') {
        throw 'Windows installer did not install the verified binary.'
    }
    $installDir = Split-Path -Parent $installed
    if ((@([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -eq $installDir })).Count -ne 1) {
        throw 'Windows installer did not add the installation directory to user PATH exactly once.'
    }
    $resolvedCommand = Get-Command vexlo.exe -ErrorAction SilentlyContinue
    if (-not $resolvedCommand -or $resolvedCommand.Source -ne $installed) {
        throw 'Windows installer did not make Vexlo available in this PowerShell session.'
    }

    & "$PSScriptRoot/install.ps1"
    if ((@([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -eq $installDir })).Count -ne 1) {
        throw 'Windows installer duplicated the PATH entry.'
    }

    [System.IO.File]::WriteAllText((Join-Path $fixture 'SHA256SUMS.txt'), "$('0' * 64)  $archiveName`n")
    $rejected = $false
    try { & "$PSScriptRoot/install.ps1" } catch { $rejected = $true }
    if (-not $rejected) {
        throw 'Windows installer accepted a bad checksum.'
    }
    Write-Host 'Windows installer tests passed.'
}
finally {
    [Environment]::SetEnvironmentVariable('Path', $originalUserPath, 'User')
    $env:Path = $originalProcessPath
    $env:LOCALAPPDATA = $originalLocalAppData
    $resolvedTestRoot = [System.IO.Path]::GetFullPath($testRoot)
    if ($resolvedTestRoot.StartsWith($tempBase, [StringComparison]::OrdinalIgnoreCase) -and
        (Split-Path -Leaf $resolvedTestRoot).StartsWith('vexlo-installer-test-')) {
        Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
