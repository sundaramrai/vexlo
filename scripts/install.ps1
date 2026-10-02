$ErrorActionPreference = 'Stop'

$runtimeArchitecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
if ($null -ne $runtimeArchitecture) {
    $architecture = $runtimeArchitecture.ToString()
} elseif ($env:PROCESSOR_ARCHITEW6432) {
    $architecture = $env:PROCESSOR_ARCHITEW6432
} else {
    $architecture = $env:PROCESSOR_ARCHITECTURE
}
if ($architecture -notin @('X64', 'AMD64', 'ARM64')) {
    throw "Unsupported Windows architecture: $architecture"
}
$arch = if ($architecture -eq 'ARM64') { 'arm64' } else { 'amd64' }
$archiveName = "vexlo-windows-$arch.zip"
$releaseBase = 'https://github.com/sundaramrai/vexlo/releases/latest/download'
$workDir = Join-Path ([System.IO.Path]::GetTempPath()) ("vexlo-install-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $workDir | Out-Null

try {
    $archivePath = Join-Path $workDir $archiveName
    $checksumPath = Join-Path $workDir 'SHA256SUMS.txt'
    Invoke-WebRequest -Uri "$releaseBase/$archiveName" -OutFile $archivePath
    Invoke-WebRequest -Uri "$releaseBase/SHA256SUMS.txt" -OutFile $checksumPath

    $matchingLines = @(Get-Content -LiteralPath $checksumPath | Where-Object { $_ -match ('^[0-9a-fA-F]{64}\s+' + [regex]::Escape($archiveName) + '$') })
    if ($matchingLines.Count -ne 1) {
        throw "Release checksum for $archiveName is missing or duplicated."
    }
    $expected = ($matchingLines[0] -split '\s+')[0].ToUpperInvariant()
    $actual = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToUpperInvariant()
    if ($actual -ne $expected) {
        throw 'Release checksum does not match.'
    }

    $extractDir = Join-Path $workDir 'extract'
    Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDir
    $source = Join-Path $extractDir "vexlo-windows-$arch.exe"
    if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
        throw 'Release archive does not contain the Vexlo client.'
    }

    $installDir = Join-Path $env:LOCALAPPDATA 'Programs\Vexlo'
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    $target = Join-Path $installDir 'vexlo.exe'
    Copy-Item -LiteralPath $source -Destination $target -Force

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $parts = @($userPath -split ';' | Where-Object { $_ })
    if ($parts -notcontains $installDir) {
        [Environment]::SetEnvironmentVariable('Path', (($parts + $installDir) -join ';'), 'User')
    }
    if (($env:Path -split ';') -notcontains $installDir) {
        $env:Path += ";$installDir"
    }
    Write-Host "Installed $target"
    Write-Host 'Run: vexlo http 3000 (new PowerShell windows will also find it)'
}
finally {
    $tempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
    $resolvedWorkDir = [System.IO.Path]::GetFullPath($workDir)
    if ($resolvedWorkDir.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -and
        (Split-Path -Leaf $resolvedWorkDir).StartsWith('vexlo-install-')) {
        Remove-Item -LiteralPath $resolvedWorkDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
