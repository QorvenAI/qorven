# Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
#
# Qorven installer for Windows — one-shot PowerShell script.
#
#   iwr -useb https://qorven.ai/install.ps1 | iex
#
# What it does:
#   1. Installs PostgreSQL via winget (if missing)
#   2. Installs pgvector extension
#   3. Creates the qorven database and role
#   4. Downloads the Qorven binary (windows/amd64)
#   5. Writes config.toml + secrets
#   6. Registers a Windows Service via NSSM (auto-start on boot)
#   7. Prints the URL and opens the browser
#
# Requirements:
#   - Windows 10 22H2+ or Windows Server 2019+
#   - PowerShell 5.1+ (pre-installed on all modern Windows)
#   - Run in an elevated (Administrator) terminal
#   - Internet access

#Requires -RunAsAdministrator
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ── CLI flags ─────────────────────────────────────────────────────────────────
$YesAll      = $args -contains '--yes'
$SkipService = $args -contains '--skip-service'

# ── configuration ─────────────────────────────────────────────────────────────
# Use if/else instead of ?? so this works on PowerShell 5.1 (built-in on Windows)
if ($env:GITHUB_OWNER)        { $GithubOwner  = $env:GITHUB_OWNER }        else { $GithubOwner  = 'qorvenai' }
if ($env:GITHUB_REPO)         { $GithubRepo   = $env:GITHUB_REPO }         else { $GithubRepo   = 'qorven' }
if ($env:RELEASE_TAG)         { $ReleaseTag   = $env:RELEASE_TAG }         else { $ReleaseTag   = 'latest' }
if ($env:QORVEN_INSTALL_DIR)  { $InstallDir   = $env:QORVEN_INSTALL_DIR }  else { $InstallDir   = 'C:\Program Files\Qorven' }
if ($env:QORVEN_CONFIG_DIR)   { $ConfigDir    = $env:QORVEN_CONFIG_DIR }   else { $ConfigDir    = 'C:\ProgramData\Qorven' }
if ($env:QORVEN_DATA_DIR)     { $DataDir      = $env:QORVEN_DATA_DIR }     else { $DataDir      = 'C:\ProgramData\Qorven\data' }
$LogDir       = "$ConfigDir\logs"
$ServiceName  = 'QorvenAI'
if ($env:PG_VERSION)          { $PgVersion    = $env:PG_VERSION }          else { $PgVersion    = '16' }
$NssmVersion  = '2.24'
$Port         = 8080   # Windows: 443 needs a cert; default to 8080 for simplicity
$ApiPort      = 4200

# ── colours ───────────────────────────────────────────────────────────────────
function Write-Step   { param($n, $total, $msg) Write-Host "`n  [$n/$total] $msg" -ForegroundColor Cyan }
function Write-Ok     { param($msg) Write-Host "  [OK] $msg" -ForegroundColor Green }
function Write-Warn   { param($msg) Write-Host "  [!!] $msg" -ForegroundColor Yellow }
function Write-Fail   { param($msg) Write-Host "  [XX] $msg" -ForegroundColor Red; exit 1 }
function Write-Info   { param($msg) Write-Host "       $msg" -ForegroundColor DarkGray }

# ── banner ────────────────────────────────────────────────────────────────────
try { Clear-Host } catch {}
Write-Host ""
Write-Host "  +-+ +-+ +-+ +-+ +-+ +-+" -ForegroundColor Blue
Write-Host "  |Q| |o| |r| |v| |e| |n|" -ForegroundColor Blue
Write-Host "  +-+ +-+ +-+ +-+ +-+ +-+" -ForegroundColor Blue
Write-Host ""
Write-Host "  Self-Hosted AI Agent Platform  --  qorven.ai" -ForegroundColor White
Write-Host ""

# ── capability notice ─────────────────────────────────────────────────────────
Write-Host "  +-- What Qorven agents can do on this machine -------------------+" -ForegroundColor Yellow
Write-Host "  |  * Browse the web and fetch external URLs                      |" -ForegroundColor Yellow
Write-Host "  |  * Read and write files on this server                         |" -ForegroundColor Yellow
Write-Host "  |  * Execute commands and run code                               |" -ForegroundColor Yellow
Write-Host "  |  * Send messages via email, Slack, Telegram                    |" -ForegroundColor Yellow
Write-Host "  |  * Run on a schedule without your active involvement           |" -ForegroundColor Yellow
Write-Host "  |  * Spend API credits (OpenAI, Anthropic, etc.) autonomously    |" -ForegroundColor Yellow
Write-Host "  |                                                                 |" -ForegroundColor Yellow
Write-Host "  |  You are responsible for securing this server and              |" -ForegroundColor Yellow
Write-Host "  |  setting agent spend limits.                                   |" -ForegroundColor Yellow
Write-Host "  +-----------------------------------------------------------------+" -ForegroundColor Yellow
Write-Host ""

if ($YesAll) {
    $answer = 'y'
} else {
    $answer = Read-Host "  Continue with installation? [y/N]"
}
if ($answer -notmatch '^[Yy]') { Write-Host "  Installation cancelled."; exit 0 }

# ── helpers ───────────────────────────────────────────────────────────────────
function Command-Exists { param($cmd) return [bool](Get-Command $cmd -ErrorAction SilentlyContinue) }

function Random-Hex {
    param($bytes)
    $buf = [byte[]]::new($bytes)
    $rng = [System.Security.Cryptography.RNGCryptoServiceProvider]::new()
    $rng.GetBytes($buf)
    $rng.Dispose()
    return ([System.BitConverter]::ToString($buf) -replace '-','').ToLower()
}

function Get-MyIP {
    try { return (Invoke-RestMethod 'https://api.ipify.org').Trim() } catch {}
    try { return (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.IPAddress -ne '127.0.0.1' -and $_.PrefixOrigin -ne 'WellKnown' } | Select-Object -First 1).IPAddress } catch {}
    return 'localhost'
}

# ── Step 1: winget / package manager ─────────────────────────────────────────
Write-Step 1 7 "Checking prerequisites"

$WingetAvail = Command-Exists 'winget'
if ($WingetAvail) {
    Write-Ok "winget found: $(winget --version)"
} else {
    Write-Warn "winget not found — will rely on pre-installed software"
}

# ── Step 2: PostgreSQL ────────────────────────────────────────────────────────
Write-Step 2 7 "PostgreSQL + pgvector"

$pgService = Get-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
if ($pgService) {
    Write-Ok "PostgreSQL $PgVersion already installed"
} elseif ($WingetAvail) {
    Write-Info "Installing PostgreSQL $PgVersion via winget..."
    winget install --id PostgreSQL.PostgreSQL.$PgVersion --silent --accept-package-agreements --accept-source-agreements
    if ($LASTEXITCODE -ne 0) { Write-Fail "PostgreSQL install failed" }
    Write-Ok "PostgreSQL $PgVersion installed"
    Start-Sleep -Seconds 3
} else {
    Write-Fail "PostgreSQL $PgVersion not found and winget is not available"
}

# Ensure service is running
$pgService = Get-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
if ($pgService -and $pgService.Status -ne 'Running') {
    Start-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}
# Re-check; if still not running after start attempt, log and continue
$pgService = Get-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
if ($pgService -and $pgService.Status -ne 'Running') {
    Write-Warn "PostgreSQL service did not start — proceeding anyway"
}

# Find psql
$PgBinDir = "C:\Program Files\PostgreSQL\$PgVersion\bin"
if (-not (Test-Path "$PgBinDir\psql.exe")) {
    # Try other common locations
    $found = Get-ChildItem 'C:\Program Files\PostgreSQL' -Filter 'psql.exe' -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($found) { $PgBinDir = $found.DirectoryName }
    else { Write-Fail "psql.exe not found — PostgreSQL may not have installed correctly" }
}
Write-Info "Using psql at $PgBinDir"

# Install pgvector — build from source (no Windows binary package exists)
Write-Info "Installing pgvector extension..."
$pgvectorDir = "$env:TEMP\pgvector"
if (-not (Test-Path $pgvectorDir)) {
    if (-not (Command-Exists 'git')) {
        if ($WingetAvail) {
            Write-Info "Installing Git via winget..."
            winget install --id Git.Git --silent --accept-package-agreements --accept-source-agreements
            $env:PATH += ";C:\Program Files\Git\cmd"
        } else {
            Write-Fail "Git not found and winget is not available — cannot build pgvector"
        }
    }
    git clone --depth 1 https://github.com/pgvector/pgvector.git $pgvectorDir 2>&1 | Out-Null
}

# Check for nmake (Visual Studio Build Tools)
$nmake = Get-Command nmake -ErrorAction SilentlyContinue
if (-not $nmake) {
    if ($WingetAvail) {
        Write-Info "Installing Visual Studio Build Tools (needed to compile pgvector)..."
        winget install --id Microsoft.VisualStudio.2022.BuildTools --silent --accept-package-agreements --accept-source-agreements `
            --override "--wait --passive --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended"
    } else {
        Write-Warn "nmake not found and winget unavailable — pgvector build may fail"
    }
    # Find nmake in VS install
    $nmakePath = Get-ChildItem 'C:\Program Files (x86)\Microsoft Visual Studio' -Filter 'nmake.exe' -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($nmakePath) { $env:PATH += ";$($nmakePath.DirectoryName)" }
}

try {
    Push-Location $pgvectorDir
    $env:PATH += ";$PgBinDir"
    nmake /f Makefile.win 2>&1 | Out-Null
    nmake /f Makefile.win install 2>&1 | Out-Null
    Write-Ok "pgvector built and installed"
} catch {
    Write-Warn "pgvector build failed — vector search will be disabled. See $env:TEMP\pgvector"
} finally {
    Pop-Location
}

# ── Step 3: Database setup ────────────────────────────────────────────────────
Write-Step 3 7 "Database setup"

$env:PATH += ";$PgBinDir"

# Run psql as postgres superuser
function Invoke-Psql {
    param([string]$Sql, [string]$Db = 'postgres')
    $result = & "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d $Db -tAc $Sql 2>&1
    return $result
}

$roleExists = Invoke-Psql "SELECT 1 FROM pg_roles WHERE rolname='qorven'"
if ($roleExists -notmatch '1') {
    Invoke-Psql "CREATE ROLE qorven LOGIN;" | Out-Null
    Write-Ok "role 'qorven' created"
} else {
    Write-Ok "role 'qorven' already exists"
}

$dbExists = Invoke-Psql "SELECT 1 FROM pg_database WHERE datname='qorven'"
if ($dbExists -notmatch '1') {
    & "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -c "CREATE DATABASE qorven OWNER qorven;" 2>&1 | Out-Null
    Write-Ok "database 'qorven' created"
} else {
    Write-Ok "database 'qorven' already exists"
}

& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d qorven -c "CREATE EXTENSION IF NOT EXISTS vector;" 2>&1 | Out-Null
Write-Ok "pgvector extension enabled"

$PG_DSN = "postgres://qorven@localhost:5432/qorven?sslmode=disable"

# ── Step 4: Directories ───────────────────────────────────────────────────────
Write-Step 4 7 "Directories"

foreach ($dir in @($InstallDir, $ConfigDir, $DataDir, $LogDir)) {
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
}
Write-Ok "Directories ready"

# ── Step 5: Qorven binary ─────────────────────────────────────────────────────
Write-Step 5 7 "Qorven binary"

$BinaryPath = "$InstallDir\qorven.exe"
$localBinary = $env:QORVEN_BINARY

if ($localBinary) {
    Copy-Item $localBinary $BinaryPath -Force
    Write-Ok "Installed from local path: $BinaryPath"
} else {
    if ($ReleaseTag -eq 'latest') {
        # Fetch all releases so pre-releases (alpha/beta) are included
        $apiUrl = "https://api.github.com/repos/$GithubOwner/$GithubRepo/releases"
        $releases = Invoke-RestMethod $apiUrl -Headers @{ 'User-Agent' = 'qorven-installer' }
        $ReleaseTag = $releases[0].tag_name
    }
    $BinaryUrl = "https://github.com/$GithubOwner/$GithubRepo/releases/download/$ReleaseTag/qorven-windows-amd64.exe"
    Write-Info "Downloading $BinaryUrl ..."
    Invoke-WebRequest -Uri $BinaryUrl -OutFile "$BinaryPath.tmp" -UseBasicParsing
    Move-Item "$BinaryPath.tmp" $BinaryPath -Force
    Write-Ok "Downloaded: $BinaryPath"
}

# Add to system PATH permanently
$sysPath = [System.Environment]::GetEnvironmentVariable('PATH', 'Machine')
if ($sysPath -notlike "*$InstallDir*") {
    [System.Environment]::SetEnvironmentVariable('PATH', "$sysPath;$InstallDir", 'Machine')
    $env:PATH += ";$InstallDir"
    Write-Info "Added $InstallDir to system PATH"
}

# ── Step 6: Config ────────────────────────────────────────────────────────────
Write-Step 6 7 "Configuration"

$ConfigPath = "$ConfigDir\config.toml"
if (Test-Path $ConfigPath) {
    Write-Warn "$ConfigPath already exists — leaving it unchanged. Delete to regenerate."
} else {
    $EncKey   = Random-Hex 32
    $AuthToken = Random-Hex 16
    $configContent = @"
# Qorven configuration — generated by install.ps1 on $(Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ')
# The encryption_key is the ONLY copy. Lose it = lose all stored secrets.

[server]
api_listen = "127.0.0.1:$ApiPort"
web_listen = "0.0.0.0:$Port"

[database]
dsn = "$PG_DSN"

[auth]
token          = "$AuthToken"
encryption_key = "$EncKey"

[server.tls]
mode = "disabled"
"@
    Set-Content -Path $ConfigPath -Value $configContent -Encoding UTF8
    # Restrict read access to current user + SYSTEM
    $acl = Get-Acl $ConfigPath
    $acl.SetAccessRuleProtection($true, $false)
    $rule1 = New-Object System.Security.AccessControl.FileSystemAccessRule("SYSTEM","FullControl","Allow")
    $rule2 = New-Object System.Security.AccessControl.FileSystemAccessRule([System.Security.Principal.WindowsIdentity]::GetCurrent().Name,"FullControl","Allow")
    $acl.AddAccessRule($rule1); $acl.AddAccessRule($rule2)
    Set-Acl $ConfigPath $acl
    Write-Ok "Wrote $ConfigPath"
}

# ── Step 7: Windows Service via NSSM ─────────────────────────────────────────
Write-Step 7 7 "Windows Service"

if ($SkipService) {
    Write-Ok "Windows Service (skipped — --skip-service flag)"
} else {

$NssmPath = "$InstallDir\nssm.exe"
if (-not (Test-Path $NssmPath)) {
    Write-Info "Downloading NSSM (service wrapper)..."
    $NssmUrl = "https://nssm.cc/release/nssm-$NssmVersion.zip"
    $NssmZip = "$env:TEMP\nssm.zip"
    Invoke-WebRequest -Uri $NssmUrl -OutFile $NssmZip -UseBasicParsing
    Expand-Archive -Path $NssmZip -DestinationPath "$env:TEMP\nssm" -Force
    # Pick the right architecture
    $nssmBin = Get-ChildItem "$env:TEMP\nssm" -Filter 'nssm.exe' -Recurse | Where-Object { $_.FullName -match 'win64' } | Select-Object -First 1
    if (-not $nssmBin) { $nssmBin = Get-ChildItem "$env:TEMP\nssm" -Filter 'nssm.exe' -Recurse | Select-Object -First 1 }
    Copy-Item $nssmBin.FullName $NssmPath -Force
    Remove-Item $NssmZip -Force
    Write-Ok "NSSM installed"
}

# Stop existing service if running
$svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($svc) {
    Write-Info "Removing existing service..."
    & $NssmPath stop $ServiceName 2>&1 | Out-Null
    & $NssmPath remove $ServiceName confirm 2>&1 | Out-Null
}

# Register service
& $NssmPath install $ServiceName $BinaryPath start 2>&1 | Out-Null
& $NssmPath set $ServiceName AppParameters "start" 2>&1 | Out-Null
& $NssmPath set $ServiceName AppDirectory $DataDir 2>&1 | Out-Null
& $NssmPath set $ServiceName AppEnvironmentExtra "QORVEN_CONFIG=$ConfigPath" 2>&1 | Out-Null
& $NssmPath set $ServiceName AppStdout "$LogDir\qorven.log" 2>&1 | Out-Null
& $NssmPath set $ServiceName AppStderr "$LogDir\qorven.log" 2>&1 | Out-Null
& $NssmPath set $ServiceName AppRotateFiles 1 2>&1 | Out-Null
& $NssmPath set $ServiceName AppRotateBytes 10485760 2>&1 | Out-Null  # 10 MB
& $NssmPath set $ServiceName Start SERVICE_AUTO_START 2>&1 | Out-Null
& $NssmPath set $ServiceName Description "Qorven AI Agent Platform" 2>&1 | Out-Null
Start-Service -Name $ServiceName
Write-Ok "Windows Service '$ServiceName' registered and started (auto-start on boot)"

} # end if (-not $SkipService)

# ── Health check ──────────────────────────────────────────────────────────────
$healthy = $false
if (-not $SkipService) {
    Write-Info "Waiting for service to become healthy..."
    for ($i = 1; $i -le 30; $i++) {
        try {
            $r = Invoke-WebRequest -Uri "http://127.0.0.1:$ApiPort/health" -UseBasicParsing -TimeoutSec 2 -ErrorAction SilentlyContinue
            if ($r.StatusCode -eq 200) { $healthy = $true; break }
        } catch {}
        Write-Host -NoNewline "."
        Start-Sleep -Seconds 2
    }
    Write-Host ""
}

$MyIP = Get-MyIP
$URL  = "http://${MyIP}:${Port}"

# ── Summary ───────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  +----------------------------------------------------------+" -ForegroundColor Green
Write-Host "  |  [OK]  Qorven installed successfully                     |" -ForegroundColor Green
Write-Host "  +----------------------------------------------------------+" -ForegroundColor Green
Write-Host "  |  Open in browser  ->  $($URL.PadRight(33))|" -ForegroundColor Green
Write-Host "  +----------------------------------------------------------+" -ForegroundColor Green
Write-Host "  |  Config:  $($ConfigPath.PadRight(47))|" -ForegroundColor Green
Write-Host "  |  Logs:    $("$LogDir\qorven.log".PadRight(47))|" -ForegroundColor Green
Write-Host "  |  Service: Get-Service $($ServiceName.PadRight(35))|" -ForegroundColor Green
Write-Host "  +----------------------------------------------------------+" -ForegroundColor Green
Write-Host ""

if ($healthy) {
    Write-Ok "API is healthy"
} else {
    Write-Warn "API did not respond — check logs: $LogDir\qorven.log"
}

Write-Host ""
Write-Host "  Verification:" -ForegroundColor White
Write-Host "    & '$BinaryPath' version" -ForegroundColor DarkGray
Write-Host "    Get-Service $ServiceName" -ForegroundColor DarkGray
Write-Host "    Invoke-WebRequest http://127.0.0.1:$ApiPort/health" -ForegroundColor DarkGray
Write-Host ""
Write-Host "  To uninstall:" -ForegroundColor DarkGray
Write-Host "    nssm stop $ServiceName; nssm remove $ServiceName confirm" -ForegroundColor DarkGray
Write-Host "    Remove-Item '$InstallDir' -Recurse" -ForegroundColor DarkGray
Write-Host "    Remove-Item '$ConfigDir' -Recurse" -ForegroundColor DarkGray
Write-Host ""

# Open browser
if (-not $SkipService) {
    try { Start-Process $URL } catch {}
}

exit 0
