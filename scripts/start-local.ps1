param(
    [string]$RegistrationToken = "change-me-dev-token",
    [int]$AppPort = 8000,
    [string]$ServerHttpAddr = ":8080",
    [string]$ServerTcpAddr = ":9000",
    [string]$BaseDomain = "localhost",
    [string]$HostUrl = "http://localhost:8080",
    [switch]$StartExampleApp,
    [string]$ExampleAppCommand = "python -m http.server",
    [switch]$RestartServer,
    [switch]$CheckBinaries = $true,
    [switch]$Headless,
    [string]$LogDir = "logs"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

function Get-PortFromAddr {
    param([string]$Addr)

    if ($Addr -match ":(\d+)$") {
        return [int]$Matches[1]
    }

    throw "Could not parse port from address '$Addr'"
}

function Test-TcpPortListening {
    param(
        [string]$Host,
        [int]$Port,
        [int]$TimeoutMs = 300
    )

    $client = New-Object System.Net.Sockets.TcpClient
    try {
        $result = $client.BeginConnect($Host, $Port, $null, $null)
        if (-not $result.AsyncWaitHandle.WaitOne($TimeoutMs)) {
            return $false
        }
        $client.EndConnect($result)
        return $true
    }
    catch {
        return $false
    }
    finally {
        $client.Close()
    }
}

function Get-OwningProcessIdsForPort {
    param([int]$Port)
    try {
        $conns = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction Stop
        return ($conns | Select-Object -ExpandProperty OwningProcess -Unique)
    }
    catch {
        return @()
    }
}

function Stop-ProcessesById {
    param([int[]]$Pids)
    foreach ($pid in $Pids) {
        try {
            Stop-Process -Id $pid -Force -ErrorAction Stop
            Write-Host "Stopped process $pid" -ForegroundColor Yellow
        }
        catch {
            Write-Host ("Failed to stop process $($pid): $($_)") -ForegroundColor Red
        }
    }
}

function Ensure-Binaries {
    param([string[]]$Bins)
    $missing = @()
    foreach ($b in $Bins) {
        if (-not (Get-Command $b -ErrorAction SilentlyContinue)) {
            $missing += $b
        }
    }
    return $missing
}

$serverCmd = @(
    "go run ./cmd/server",
    "--http-addr $ServerHttpAddr",
    "--tcp-addr $ServerTcpAddr",
    "--base-domain $BaseDomain",
    "--host-url $HostUrl",
    "--registration-token $RegistrationToken"
) -join " "

$clientCmd = @(
    "go run ./cmd/client",
    "--server 127.0.0.1$ServerTcpAddr",
    "--register-token $RegistrationToken",
    "$AppPort"
) -join " "

Write-Host "Starting Vexlo local stack..." -ForegroundColor Cyan

$tcpPort = Get-PortFromAddr -Addr $ServerTcpAddr
$serverAlreadyRunning = Test-TcpPortListening -Host "127.0.0.1" -Port $tcpPort

# Binary checks
if ($CheckBinaries) {
    $requiredBinaries = @('go', 'powershell')
    if ($StartExampleApp) { $requiredBinaries += 'python' }
    $missing = Ensure-Binaries -Bins $requiredBinaries
    if ($missing.Count -gt 0) {
        Write-Host "Missing required binaries: $($missing -join ', ')" -ForegroundColor Red
        Write-Host "Please install them or adjust PATH before running the script." -ForegroundColor Red
        exit 1
    }
}

# If requested, stop existing server processes
if ($serverAlreadyRunning -and $RestartServer) {
    $pids = Get-OwningProcessIdsForPort -Port $tcpPort
    if ($pids.Count -gt 0) {
        Write-Host ("Stopping existing server processes listening on port $($tcpPort): $($pids -join ', ')") -ForegroundColor Yellow
        Stop-ProcessesById -Pids $pids
        Start-Sleep -Seconds 1
        # re-evaluate
        $serverAlreadyRunning = Test-TcpPortListening -Host "127.0.0.1" -Port $tcpPort
    }
}

if ($StartExampleApp) {
    $appCmd = "$ExampleAppCommand $AppPort"
    $pw = (Get-Command powershell -ErrorAction SilentlyContinue).Source
    $appArgs = @()
    if (-not $Headless) { $appArgs += '-NoExit' }
    $appArgs += '-Command'
    $appArgs += $appCmd
    $appOut = $null; $appErr = $null
    if ($LogDir) {
        New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
        $appOut = Join-Path $LogDir "app-$(Get-Date -Format 'yyyyMMdd-HHmmss').log"
        $appErr = Join-Path $LogDir "app-$(Get-Date -Format 'yyyyMMdd-HHmmss').err.log"
    }
    $startSplat = @{ FilePath = $pw; WorkingDirectory = $repoRoot; ArgumentList = $appArgs; WindowStyle = (if ($Headless) { 'Hidden' } else { 'Normal' }) }
    if ($appOut) { $startSplat.RedirectStandardOutput = $appOut; $startSplat.RedirectStandardError = $appErr }
    Start-Process @startSplat | Out-Null
    Write-Host "Started example app terminal on port $AppPort" -ForegroundColor Green
}
if ($serverAlreadyRunning) {
    Write-Host "Server appears to already be listening on $ServerTcpAddr; skipping new server launch." -ForegroundColor Yellow
}
else {
    $pw = (Get-Command powershell -ErrorAction SilentlyContinue).Source
    $serverArgs = @()
    if (-not $Headless) { $serverArgs += '-NoExit' }
    $serverArgs += '-Command'
    $serverArgs += $serverCmd
    $serverOut = $null; $serverErr = $null
    if ($LogDir) {
        New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
        $serverOut = Join-Path $LogDir "server-$(Get-Date -Format 'yyyyMMdd-HHmmss').log"
        $serverErr = Join-Path $LogDir "server-$(Get-Date -Format 'yyyyMMdd-HHmmss').err.log"
    }
    $startSplat = @{ FilePath = $pw; WorkingDirectory = $repoRoot; ArgumentList = $serverArgs; WindowStyle = (if ($Headless) { 'Hidden' } else { 'Normal' }) }
    if ($serverOut) { $startSplat.RedirectStandardOutput = $serverOut; $startSplat.RedirectStandardError = $serverErr }
    Start-Process @startSplat | Out-Null
    Start-Sleep -Seconds 1
}

$pw = (Get-Command powershell -ErrorAction SilentlyContinue).Source
$clientArgs = @()
if (-not $Headless) { $clientArgs += '-NoExit' }
$clientArgs += '-Command'
$clientArgs += $clientCmd
$clientOut = $null; $clientErr = $null
if ($LogDir) {
    New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
    $clientOut = Join-Path $LogDir "client-$(Get-Date -Format 'yyyyMMdd-HHmmss').log"
    $clientErr = Join-Path $LogDir "client-$(Get-Date -Format 'yyyyMMdd-HHmmss').err.log"
}
$startSplat = @{ FilePath = $pw; WorkingDirectory = $repoRoot; ArgumentList = $clientArgs; WindowStyle = (if ($Headless) { 'Hidden' } else { 'Normal' }) }
if ($clientOut) { $startSplat.RedirectStandardOutput = $clientOut; $startSplat.RedirectStandardError = $clientErr }
Start-Process @startSplat | Out-Null

if ($serverAlreadyRunning) {
    Write-Host "Started client terminal (reused existing server)." -ForegroundColor Green
}
else {
    Write-Host "Started server and client terminals." -ForegroundColor Green
}
Write-Host "Health check:  curl.exe http://localhost:8080/healthz"
Write-Host "Dashboard:     http://localhost:8080"
Write-Host "Tunnel URL is printed by the client terminal after registration."
