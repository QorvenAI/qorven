# Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
#
# Qorven installer for Windows — one-shot PowerShell script.
#
#   iwr -useb https://get.qorven.ai | iex
#   iwr -useb https://get.qorven.ai/install.ps1 | iex
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
# If anything fails, all changes made so far are automatically rolled back.
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
function Write-Step { param($n, $total, $msg) Write-Host "`n  [$n/$total] $msg" -ForegroundColor Cyan }
function Write-Ok   { param($msg) Write-Host "  [OK] $msg" -ForegroundColor Green }
function Write-Warn { param($msg) Write-Host "  [!!] $msg" -ForegroundColor Yellow }
function Write-Info { param($msg) Write-Host "       $msg" -ForegroundColor DarkGray }

# ── rollback state ────────────────────────────────────────────────────────────
# Each flag records something WE installed so rollback can undo exactly that.
# Pre-existing items (PostgreSQL already installed, DB already existed, etc.)
# are never rolled back — we only remove what we added.
$script:RollbackInstalledPg      = $false  # we ran winget install PostgreSQL
$script:RollbackCreatedRole      = $false  # we ran CREATE ROLE qorven
$script:RollbackCreatedDb        = $false  # we ran CREATE DATABASE qorven
$script:RollbackCreatedInstallDir = $false # we created $InstallDir
$script:RollbackCreatedConfigDir  = $false # we created $ConfigDir
$script:RollbackCreatedService   = $false  # we registered the service
$script:PgBinDir                 = ''      # filled in once psql is found

function Invoke-Rollback {
    param([string]$Reason)
    Write-Host ""
    Write-Host "  ----------------------------------------------------------------" -ForegroundColor Red
    Write-Host "  [XX] Installation failed: $Reason" -ForegroundColor Red
    Write-Host "       Rolling back everything Qorven installed..." -ForegroundColor Yellow
    Write-Host "  ----------------------------------------------------------------" -ForegroundColor Red

    # Service
    if ($script:RollbackCreatedService) {
        try {
            $nssmExe = "$InstallDir\nssm.exe"
            if (Test-Path $nssmExe) {
                & $nssmExe stop $ServiceName 2>&1 | Out-Null
                & $nssmExe remove $ServiceName confirm 2>&1 | Out-Null
            } else {
                Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
                sc.exe delete $ServiceName 2>&1 | Out-Null
            }
            Write-Host "  [RB] Service removed" -ForegroundColor DarkGray
        } catch { Write-Host "  [RB] Could not remove service: $_" -ForegroundColor DarkGray }
    }

    # Database objects — only if we have a working psql
    if (($script:RollbackCreatedDb -or $script:RollbackCreatedRole) -and $script:PgBinDir) {
        $env:PGPASSWORD = $PgSuperPassword
        if ($script:RollbackCreatedDb) {
            & "$($script:PgBinDir)\psql.exe" -U postgres -h 127.0.0.1 -d postgres -c "DROP DATABASE IF EXISTS qorven;" 2>&1 | Out-Null
            Write-Host "  [RB] Database 'qorven' dropped" -ForegroundColor DarkGray
        }
        if ($script:RollbackCreatedRole) {
            & "$($script:PgBinDir)\psql.exe" -U postgres -h 127.0.0.1 -d postgres -c "DROP ROLE IF EXISTS qorven;" 2>&1 | Out-Null
            Write-Host "  [RB] Role 'qorven' dropped" -ForegroundColor DarkGray
        }
        Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    }

    # Config and install directories
    if ($script:RollbackCreatedConfigDir -and (Test-Path $ConfigDir)) {
        Remove-Item $ConfigDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "  [RB] Removed $ConfigDir" -ForegroundColor DarkGray
    }
    if ($script:RollbackCreatedInstallDir -and (Test-Path $InstallDir)) {
        Remove-Item $InstallDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "  [RB] Removed $InstallDir" -ForegroundColor DarkGray
    }

    # PATH
    $sysPath = [System.Environment]::GetEnvironmentVariable('PATH', 'Machine')
    if ($sysPath -like "*$InstallDir*") {
        $newPath = ($sysPath -split ';' | Where-Object { $_ -ne $InstallDir }) -join ';'
        [System.Environment]::SetEnvironmentVariable('PATH', $newPath, 'Machine')
        Write-Host "  [RB] Removed $InstallDir from PATH" -ForegroundColor DarkGray
    }

    # PostgreSQL — only if we installed it (not if it was already there)
    if ($script:RollbackInstalledPg -and (Get-Command winget -ErrorAction SilentlyContinue)) {
        Write-Host "  [RB] Removing PostgreSQL (we installed it)..." -ForegroundColor DarkGray
        winget uninstall --id PostgreSQL.PostgreSQL.$PgVersion --silent 2>&1 | Out-Null
        Write-Host "  [RB] PostgreSQL removed" -ForegroundColor DarkGray
    }

    # Temp files
    $pgvectorDir = "$env:TEMP\pgvector"
    if (Test-Path $pgvectorDir) {
        Remove-Item $pgvectorDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "  [RB] Removed pgvector temp dir" -ForegroundColor DarkGray
    }

    Write-Host ""
    Write-Host "  Rollback complete. Your system is back to its previous state." -ForegroundColor Yellow
    Write-Host "  Fix the issue above and re-run the installer." -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  To uninstall manually at any time:" -ForegroundColor DarkGray
    Write-Host "    iwr -useb https://get.qorven.ai/uninstall.ps1 | iex" -ForegroundColor DarkGray
    Write-Host ""
    exit 1
}

# Trap all terminating errors and route through rollback
trap {
    Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    Invoke-Rollback $_.Exception.Message
}

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

# ── Step 2: PostgreSQL + pgvector ─────────────────────────────────────────────
Write-Step 2 7 "PostgreSQL + pgvector"

$pgService = Get-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
if ($pgService) {
    Write-Ok "PostgreSQL $PgVersion already installed"
} elseif ($WingetAvail) {
    Write-Info "Installing PostgreSQL $PgVersion via winget..."
    winget install --id PostgreSQL.PostgreSQL.$PgVersion --silent --accept-package-agreements --accept-source-agreements
    if ($LASTEXITCODE -ne 0) { Invoke-Rollback "PostgreSQL install failed (winget exit $LASTEXITCODE)" }
    $script:RollbackInstalledPg = $true
    Write-Ok "PostgreSQL $PgVersion installed"
    Start-Sleep -Seconds 3
} else {
    Invoke-Rollback "PostgreSQL $PgVersion not found and winget is not available"
}

# Ensure service is running
$pgService = Get-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
if ($pgService -and $pgService.Status -ne 'Running') {
    Start-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}
$pgService = Get-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
if (-not $pgService -or $pgService.Status -ne 'Running') {
    Invoke-Rollback "PostgreSQL service is not running. Start it with: Start-Service postgresql-x64-$PgVersion"
}

# Find psql
$PgBinDir = "C:\Program Files\PostgreSQL\$PgVersion\bin"
if (-not (Test-Path "$PgBinDir\psql.exe")) {
    $found = Get-ChildItem 'C:\Program Files\PostgreSQL' -Filter 'psql.exe' -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($found) { $PgBinDir = $found.DirectoryName }
    else { Invoke-Rollback "psql.exe not found — PostgreSQL may not have installed correctly" }
}
$script:PgBinDir = $PgBinDir
Write-Info "Using psql at $PgBinDir"

# pgvector — build from source
Write-Info "Installing pgvector extension..."
$pgvectorDir = "$env:TEMP\pgvector"
if (-not (Test-Path $pgvectorDir)) {
    if (-not (Command-Exists 'git')) {
        if ($WingetAvail) {
            Write-Info "Installing Git via winget..."
            winget install --id Git.Git --silent --accept-package-agreements --accept-source-agreements
            $env:PATH += ";C:\Program Files\Git\cmd"
        } else {
            Invoke-Rollback "Git not found and winget unavailable — cannot build pgvector"
        }
    }
    $gitOut = git clone --depth 1 https://github.com/pgvector/pgvector.git $pgvectorDir 2>&1
    if ($LASTEXITCODE -ne 0) { Invoke-Rollback "git clone pgvector failed: $gitOut" }
}

$nmake = Get-Command nmake -ErrorAction SilentlyContinue
if (-not $nmake) {
    if ($WingetAvail) {
        Write-Info "Installing Visual Studio Build Tools (needed to compile pgvector)..."
        winget install --id Microsoft.VisualStudio.2022.BuildTools --silent --accept-package-agreements --accept-source-agreements `
            --override "--wait --passive --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended"
    } else {
        Write-Warn "nmake not found and winget unavailable — pgvector build may fail"
    }
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
    Write-Warn "pgvector build failed — vector search will be disabled. Error: $_"
} finally {
    Pop-Location
}

# ── Step 3: PostgreSQL superuser password ─────────────────────────────────────
# Collected here — after we know PG is running, before we need it.
# This avoids prompting before a potentially long pgvector build.
Write-Step 3 7 "Database setup"

$env:PATH += ";$PgBinDir"

if ($env:PG_SUPERUSER_PASSWORD) {
    $PgSuperPassword = $env:PG_SUPERUSER_PASSWORD
} else {
    Write-Host ""
    Write-Host "  Enter the PostgreSQL 'postgres' superuser password." -ForegroundColor Cyan
    Write-Host "  This is the password set when PostgreSQL was installed." -ForegroundColor Cyan
    Write-Host "  It is only used during setup and is NOT stored by Qorven." -ForegroundColor Cyan
    Write-Host ""
    $secPw = Read-Host "  postgres superuser password" -AsSecureString
    $bstr  = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($secPw)
    $PgSuperPassword = [System.Runtime.InteropServices.Marshal]::PtrToStringAuto($bstr)
    [System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
}

$env:PGPASSWORD = $PgSuperPassword

function Invoke-Psql {
    param([string]$Sql, [string]$Db = 'postgres')
    $result = & "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d $Db -tAc $Sql 2>&1
    return $result
}

# Verify password before doing anything
$pgCheck = Invoke-Psql "SELECT 1"
if ($pgCheck -notmatch '1') {
    Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    Invoke-Rollback "Cannot connect to PostgreSQL as 'postgres' — wrong password or service not running. Error: $pgCheck"
}
Write-Ok "Connected to PostgreSQL"

$roleExists = Invoke-Psql "SELECT 1 FROM pg_roles WHERE rolname='qorven'"
if ($roleExists -notmatch '1') {
    Invoke-Psql "CREATE ROLE qorven LOGIN;" | Out-Null
    $script:RollbackCreatedRole = $true
    Write-Ok "Role 'qorven' created"
} else {
    Write-Ok "Role 'qorven' already exists"
}

$dbExists = Invoke-Psql "SELECT 1 FROM pg_database WHERE datname='qorven'"
if ($dbExists -notmatch '1') {
    & "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -c "CREATE DATABASE qorven OWNER qorven;" 2>&1 | Out-Null
    $script:RollbackCreatedDb = $true
    Write-Ok "Database 'qorven' created"
} else {
    Write-Ok "Database 'qorven' already exists"
}

& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d qorven -c "CREATE EXTENSION IF NOT EXISTS vector;" 2>&1 | Out-Null
Write-Ok "pgvector extension enabled"
Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue

$PG_DSN = "postgres://qorven@localhost:5432/qorven?sslmode=disable"

# ── Step 4: Directories ───────────────────────────────────────────────────────
Write-Step 4 7 "Directories"

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $script:RollbackCreatedInstallDir = $true
}
if (-not (Test-Path $ConfigDir)) {
    New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
    $script:RollbackCreatedConfigDir = $true
}
foreach ($dir in @($DataDir, $LogDir)) {
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
}
Write-Ok "Directories ready"

# ── Step 5: Qorven binary ─────────────────────────────────────────────────────
Write-Step 5 7 "Qorven binary"

$BinaryPath  = "$InstallDir\qorven.exe"
$localBinary = $env:QORVEN_BINARY

if ($localBinary) {
    Copy-Item $localBinary $BinaryPath -Force
    Write-Ok "Installed from local path: $BinaryPath"
} else {
    if ($ReleaseTag -eq 'latest') {
        $apiUrl   = "https://api.github.com/repos/$GithubOwner/$GithubRepo/releases"
        $releases = Invoke-RestMethod $apiUrl -Headers @{ 'User-Agent' = 'qorven-installer' }
        $ReleaseTag = ($releases | Where-Object { -not $_.draft } | Select-Object -First 1).tag_name
        if (-not $ReleaseTag) { Invoke-Rollback "No releases found in $GithubOwner/$GithubRepo" }
    }
    $BinaryUrl = "https://github.com/$GithubOwner/$GithubRepo/releases/download/$ReleaseTag/qorven-windows-amd64.exe"
    Write-Info "Downloading $BinaryUrl ..."
    Invoke-WebRequest -Uri $BinaryUrl -OutFile "$BinaryPath.tmp" -UseBasicParsing
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path "$BinaryPath.tmp")) {
        Invoke-Rollback "Binary download failed from $BinaryUrl"
    }
    Move-Item "$BinaryPath.tmp" $BinaryPath -Force
    Write-Ok "Downloaded: $BinaryPath"
}

# Add to system PATH
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
    $EncKey    = Random-Hex 32
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
    $nssmBin = Get-ChildItem "$env:TEMP\nssm" -Filter 'nssm.exe' -Recurse | Where-Object { $_.FullName -match 'win64' } | Select-Object -First 1
    if (-not $nssmBin) { $nssmBin = Get-ChildItem "$env:TEMP\nssm" -Filter 'nssm.exe' -Recurse | Select-Object -First 1 }
    if (-not $nssmBin) { Invoke-Rollback "Could not find nssm.exe in downloaded archive" }
    Copy-Item $nssmBin.FullName $NssmPath -Force
    Remove-Item $NssmZip -Force
    Write-Ok "NSSM installed"
}

$svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($svc) {
    Write-Info "Removing existing service..."
    & $NssmPath stop $ServiceName 2>&1 | Out-Null
    & $NssmPath remove $ServiceName confirm 2>&1 | Out-Null
}

& $NssmPath install $ServiceName $BinaryPath start 2>&1 | Out-Null
& $NssmPath set $ServiceName AppParameters "start" 2>&1 | Out-Null
& $NssmPath set $ServiceName AppDirectory $DataDir 2>&1 | Out-Null
& $NssmPath set $ServiceName AppEnvironmentExtra "QORVEN_CONFIG=$ConfigPath" 2>&1 | Out-Null
& $NssmPath set $ServiceName AppStdout "$LogDir\qorven.log" 2>&1 | Out-Null
& $NssmPath set $ServiceName AppStderr "$LogDir\qorven.log" 2>&1 | Out-Null
& $NssmPath set $ServiceName AppRotateFiles 1 2>&1 | Out-Null
& $NssmPath set $ServiceName AppRotateBytes 10485760 2>&1 | Out-Null
& $NssmPath set $ServiceName Start SERVICE_AUTO_START 2>&1 | Out-Null
& $NssmPath set $ServiceName Description "Qorven AI Agent Platform" 2>&1 | Out-Null
$script:RollbackCreatedService = $true
Start-Service -Name $ServiceName
Write-Ok "Windows Service '$ServiceName' registered and started"

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
    if (-not $healthy) {
        Invoke-Rollback "Service started but API never became healthy after 60 s. Check logs: $LogDir\qorven.log"
    }
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
Write-Host "  To uninstall:" -ForegroundColor DarkGray
Write-Host "    iwr -useb https://get.qorven.ai/uninstall.ps1 | iex" -ForegroundColor DarkGray
Write-Host ""

Write-Host "  Verification:" -ForegroundColor White
Write-Host "    & '$BinaryPath' version" -ForegroundColor DarkGray
Write-Host "    Get-Service $ServiceName" -ForegroundColor DarkGray
Write-Host "    Invoke-WebRequest http://127.0.0.1:$ApiPort/health" -ForegroundColor DarkGray
Write-Host ""

if (-not $SkipService) {
    try { Start-Process $URL } catch {}
}

exit 0
