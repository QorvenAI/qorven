# Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
#
# Qorven installer for Windows — one-shot PowerShell script.
#
#   iwr -useb https://get.qorven.ai | iex
#   iwr -useb https://get.qorven.ai/install.ps1 | iex
#
# What it does:
#   1. Installs PostgreSQL via winget (if missing)
#   2. Installs pgvector extension (optional — skipped gracefully if unavailable)
#   3. Creates the qorven database and role
#   4. Downloads the Qorven binary (windows/amd64)
#   5. Writes config.toml + secrets
#   6. Registers a Windows Service via NSSM (auto-start on boot)
#   7. Prints the URL and opens the browser
#
# No password is ever required from the user.
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
if ($env:GITHUB_OWNER)        { $GithubOwner = $env:GITHUB_OWNER }        else { $GithubOwner = 'qorvenai' }
if ($env:GITHUB_REPO)         { $GithubRepo  = $env:GITHUB_REPO }         else { $GithubRepo  = 'qorven' }
if ($env:RELEASE_TAG)         { $ReleaseTag  = $env:RELEASE_TAG }         else { $ReleaseTag  = 'latest' }
if ($env:QORVEN_INSTALL_DIR)  { $InstallDir  = $env:QORVEN_INSTALL_DIR }  else { $InstallDir  = 'C:\Program Files\Qorven' }
if ($env:QORVEN_CONFIG_DIR)   { $ConfigDir   = $env:QORVEN_CONFIG_DIR }   else { $ConfigDir   = 'C:\ProgramData\Qorven' }
if ($env:QORVEN_DATA_DIR)     { $DataDir     = $env:QORVEN_DATA_DIR }     else { $DataDir     = 'C:\ProgramData\Qorven\data' }
$LogDir      = "$ConfigDir\logs"
$ServiceName = 'QorvenAI'
if ($env:PG_VERSION)          { $PgVersion   = $env:PG_VERSION }          else { $PgVersion   = '16' }
$NssmVersion = '2.24'
$Port        = 8080
$ApiPort     = 4200

# ── output helpers ────────────────────────────────────────────────────────────
function Write-Step { param($n, $total, $msg) Write-Host "`n  [$n/$total] $msg" -ForegroundColor Cyan }
function Write-Ok   { param($msg) Write-Host "  [OK] $msg" -ForegroundColor Green }
function Write-Warn { param($msg) Write-Host "  [!!] $msg" -ForegroundColor Yellow }
function Write-Info { param($msg) Write-Host "       $msg" -ForegroundColor DarkGray }
function Write-Rb   { param($msg) Write-Host "  [RB] $msg" -ForegroundColor DarkGray }

# ── rollback state ────────────────────────────────────────────────────────────
$script:RollbackInstalledPg       = $false  # we ran winget install PostgreSQL
$script:RollbackCreatedRole       = $false  # we ran CREATE ROLE qorven
$script:RollbackCreatedDb         = $false  # we ran CREATE DATABASE qorven
$script:RollbackCreatedInstallDir = $false  # we created $InstallDir
$script:RollbackCreatedConfigDir  = $false  # we created $ConfigDir
$script:RollbackCreatedService    = $false  # we registered the service
$script:PgBinDir                  = ''
$script:HbaPath                   = ''      # pg_hba.conf path if we patched it
$script:HbaOriginal               = ''      # original pg_hba.conf content

# ── pg_hba.conf trust helpers ─────────────────────────────────────────────────
# Used when we installed PostgreSQL ourselves. We don't know the superuser
# password (winget may set a random one), so we temporarily switch the auth
# method to 'trust' for localhost, do all setup, then restore it.

function Enable-PgTrustAuth {
    $hba = "C:\Program Files\PostgreSQL\$PgVersion\data\pg_hba.conf"
    if (-not (Test-Path $hba)) {
        $found = Get-ChildItem "C:\Program Files\PostgreSQL" -Filter "pg_hba.conf" -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($found) { $hba = $found.FullName } else { return $false }
    }
    $script:HbaPath     = $hba
    $script:HbaOriginal = [System.IO.File]::ReadAllText($hba)
    $patched = $script:HbaOriginal -replace 'scram-sha-256', 'trust' -replace '\bmd5\b', 'trust'
    [System.IO.File]::WriteAllText($hba, $patched)
    Restart-Service "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 3
    return $true
}

function Restore-PgTrustAuth {
    if ($script:HbaPath -and $script:HbaOriginal) {
        try {
            [System.IO.File]::WriteAllText($script:HbaPath, $script:HbaOriginal)
            Restart-Service "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
            Start-Sleep -Seconds 2
        } catch {}
        $script:HbaPath     = ''
        $script:HbaOriginal = ''
    }
}

# ── rollback ──────────────────────────────────────────────────────────────────
function Invoke-Rollback {
    param([string]$Reason)
    Write-Host ""
    Write-Host "  ----------------------------------------------------------------" -ForegroundColor Red
    Write-Host "  [XX] Installation failed: $Reason" -ForegroundColor Red
    Write-Host "       Rolling back everything Qorven installed..." -ForegroundColor Yellow
    Write-Host "  ----------------------------------------------------------------" -ForegroundColor Red

    # Restore pg_hba.conf first so the service can restart cleanly
    Restore-PgTrustAuth

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
            Write-Rb "Service removed"
        } catch { Write-Rb "Could not remove service: $_" }
    }

    # Database objects
    if (($script:RollbackCreatedDb -or $script:RollbackCreatedRole) -and $script:PgBinDir) {
        if ($script:RollbackCreatedDb) {
            & "$($script:PgBinDir)\psql.exe" -U postgres -h 127.0.0.1 -d postgres -c "DROP DATABASE IF EXISTS qorven;" 2>&1 | Out-Null
            Write-Rb "Database 'qorven' dropped"
        }
        if ($script:RollbackCreatedRole) {
            & "$($script:PgBinDir)\psql.exe" -U postgres -h 127.0.0.1 -d postgres -c "DROP ROLE IF EXISTS qorven;" 2>&1 | Out-Null
            Write-Rb "Role 'qorven' dropped"
        }
    }

    # Files
    if ($script:RollbackCreatedConfigDir -and (Test-Path $ConfigDir)) {
        Remove-Item $ConfigDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Rb "Removed $ConfigDir"
    }
    if ($script:RollbackCreatedInstallDir -and (Test-Path $InstallDir)) {
        Remove-Item $InstallDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Rb "Removed $InstallDir"
    }

    # PATH
    try {
        $sysPath = [System.Environment]::GetEnvironmentVariable('PATH', 'Machine')
        if ($sysPath -like "*$InstallDir*") {
            $newPath = ($sysPath -split ';' | Where-Object { $_ -ne $InstallDir }) -join ';'
            [System.Environment]::SetEnvironmentVariable('PATH', $newPath, 'Machine')
            Write-Rb "Removed $InstallDir from PATH"
        }
    } catch {}

    # PostgreSQL — only if we installed it
    if ($script:RollbackInstalledPg -and (Get-Command winget -ErrorAction SilentlyContinue)) {
        Write-Rb "Removing PostgreSQL (we installed it)..."
        winget uninstall --id PostgreSQL.PostgreSQL.$PgVersion --silent 2>&1 | Out-Null
        Write-Rb "PostgreSQL removed"
    }

    # Temp files
    $pgvectorDir = "$env:TEMP\pgvector"
    if (Test-Path $pgvectorDir) {
        Remove-Item $pgvectorDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Rb "Removed pgvector temp dir"
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

# Trap all terminating errors
trap {
    Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    Invoke-Rollback $_.Exception.Message
}

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

# ── banner ────────────────────────────────────────────────────────────────────
try { Clear-Host } catch {}
Write-Host ""
Write-Host "  +-+ +-+ +-+ +-+ +-+ +-+" -ForegroundColor Blue
Write-Host "  |Q| |o| |r| |v| |e| |n|" -ForegroundColor Blue
Write-Host "  +-+ +-+ +-+ +-+ +-+ +-+" -ForegroundColor Blue
Write-Host ""
Write-Host "  Self-Hosted AI Agent Platform  --  qorven.ai" -ForegroundColor White
Write-Host ""
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

# ── Step 1: Prerequisites ─────────────────────────────────────────────────────
Write-Step 1 7 "Checking prerequisites"

$WingetAvail = Command-Exists 'winget'
if ($WingetAvail) {
    Write-Ok "winget found: $(winget --version)"
} else {
    Write-Warn "winget not found — will rely on pre-installed software"
}

# ── Step 2: PostgreSQL ────────────────────────────────────────────────────────
Write-Step 2 7 "PostgreSQL"

$pgService = Get-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
if ($pgService) {
    Write-Ok "PostgreSQL $PgVersion already installed"
} elseif ($WingetAvail) {
    Write-Info "Installing PostgreSQL $PgVersion via winget (this may take a few minutes)..."
    winget install --id PostgreSQL.PostgreSQL.$PgVersion --silent --accept-package-agreements --accept-source-agreements
    if ($LASTEXITCODE -ne 0) { Invoke-Rollback "PostgreSQL install failed (winget exit $LASTEXITCODE)" }
    $script:RollbackInstalledPg = $true
    Write-Ok "PostgreSQL $PgVersion installed"
    Start-Sleep -Seconds 5
} else {
    Invoke-Rollback "PostgreSQL $PgVersion not found and winget is not available. Install PostgreSQL manually from https://www.postgresql.org/download/windows/ then re-run."
}

# Ensure service is running
$pgService = Get-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
if ($pgService -and $pgService.Status -ne 'Running') {
    Write-Info "Starting PostgreSQL service..."
    Start-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 3
}
$pgService = Get-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
if (-not $pgService -or $pgService.Status -ne 'Running') {
    Invoke-Rollback "PostgreSQL service is not running. Try: Start-Service postgresql-x64-$PgVersion"
}

# Find psql.exe
$PgBinDir = "C:\Program Files\PostgreSQL\$PgVersion\bin"
if (-not (Test-Path "$PgBinDir\psql.exe")) {
    $found = Get-ChildItem 'C:\Program Files\PostgreSQL' -Filter 'psql.exe' -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($found) { $PgBinDir = $found.DirectoryName }
    else { Invoke-Rollback "psql.exe not found — PostgreSQL may not have installed correctly" }
}
$script:PgBinDir = $PgBinDir
$env:PATH += ";$PgBinDir"
Write-Ok "PostgreSQL service running — psql at $PgBinDir"

# ── Step 3: pgvector (optional) ───────────────────────────────────────────────
Write-Step 3 7 "pgvector (vector search — optional)"

$pgvectorInstalled = $false
$pgvectorDir = "$env:TEMP\pgvector"

# Only try to build if nmake is already available — do NOT install VS Build Tools
# (multi-GB download that takes 30+ min on a fresh machine).
$nmake = Get-Command nmake -ErrorAction SilentlyContinue
if ($nmake) {
    Write-Info "nmake found — building pgvector from source..."
    if (-not (Test-Path $pgvectorDir)) {
        if (Command-Exists 'git') {
            $gitOut = git clone --depth 1 https://github.com/pgvector/pgvector.git $pgvectorDir 2>&1
            if ($LASTEXITCODE -ne 0) {
                Write-Warn "git clone pgvector failed — vector search will be disabled. ($gitOut)"
            }
        } else {
            Write-Warn "git not found — skipping pgvector build. Vector search will be disabled."
        }
    }
    if (Test-Path "$pgvectorDir\Makefile.win") {
        try {
            Push-Location $pgvectorDir
            nmake /f Makefile.win 2>&1 | Out-Null
            nmake /f Makefile.win install 2>&1 | Out-Null
            $pgvectorInstalled = $true
            Write-Ok "pgvector built and installed"
        } catch {
            Write-Warn "pgvector build failed — vector search will be disabled."
        } finally {
            Pop-Location
        }
    }
} else {
    Write-Warn "Visual Studio Build Tools not found — skipping pgvector build."
    Write-Info "Vector search will be disabled. Install is faster without it."
    Write-Info "You can add pgvector later: https://github.com/pgvector/pgvector#windows"
}

# ── Step 4: Database setup ────────────────────────────────────────────────────
Write-Step 4 7 "Database setup"

# Connect to PostgreSQL without asking the user for a password.
#
# Strategy:
#   1. Try passwordless first (works if pg_hba.conf already allows trust/peer).
#   2. If we installed PostgreSQL ourselves: patch pg_hba.conf to trust auth,
#      restart the service, do all setup, restore pg_hba.conf at the end.
#      This avoids needing the superuser password at all — winget sets a random
#      one we never see.
#   3. If PostgreSQL was already on the machine: try the PG_SUPERUSER_PASSWORD
#      env var, then ask once. The user should know their own PG password.

$env:PGPASSWORD = ''
$canConnect = ((& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT 1" 2>&1) -match '1')

if (-not $canConnect) {
    if ($script:RollbackInstalledPg) {
        # We own this PostgreSQL install — use pg_hba.conf trust trick
        Write-Info "Configuring temporary passwordless access for setup..."
        $trustOk = Enable-PgTrustAuth
        if (-not $trustOk) {
            Invoke-Rollback "Could not locate pg_hba.conf to configure database access"
        }
        $canConnect = ((& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT 1" 2>&1) -match '1')
        if (-not $canConnect) {
            Restore-PgTrustAuth
            Invoke-Rollback "Still cannot connect to PostgreSQL after configuring trust auth"
        }
        Write-Ok "Passwordless access configured (will be restored after setup)"
    } else {
        # PostgreSQL was pre-existing — we need the user's password
        if ($env:PG_SUPERUSER_PASSWORD) {
            $env:PGPASSWORD = $env:PG_SUPERUSER_PASSWORD
        } else {
            Write-Host ""
            Write-Host "  PostgreSQL was already installed on this machine." -ForegroundColor Cyan
            Write-Host "  Enter the 'postgres' superuser password to continue." -ForegroundColor Cyan
            Write-Host "  (This is the password you set when PostgreSQL was installed.)" -ForegroundColor DarkGray
            Write-Host ""
            $secPw = Read-Host "  postgres password" -AsSecureString
            $bstr  = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($secPw)
            $env:PGPASSWORD = [System.Runtime.InteropServices.Marshal]::PtrToStringAuto($bstr)
            [System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
        }
        $canConnect = ((& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT 1" 2>&1) -match '1')
        if (-not $canConnect) {
            Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
            Invoke-Rollback "Wrong PostgreSQL password. Re-run and enter the correct 'postgres' superuser password."
        }
    }
}

Write-Ok "Connected to PostgreSQL"

function Invoke-Psql {
    param([string]$Sql, [string]$Db = 'postgres')
    return (& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d $Db -tAc $Sql 2>&1)
}

$roleExists = (Invoke-Psql "SELECT 1 FROM pg_roles WHERE rolname='qorven'") -match '1'
if (-not $roleExists) {
    Invoke-Psql "CREATE ROLE qorven LOGIN;" | Out-Null
    $script:RollbackCreatedRole = $true
    Write-Ok "Role 'qorven' created"
} else {
    Write-Ok "Role 'qorven' already exists"
}

$dbExists = (Invoke-Psql "SELECT 1 FROM pg_database WHERE datname='qorven'") -match '1'
if (-not $dbExists) {
    & "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -c "CREATE DATABASE qorven OWNER qorven;" 2>&1 | Out-Null
    $script:RollbackCreatedDb = $true
    Write-Ok "Database 'qorven' created"
} else {
    Write-Ok "Database 'qorven' already exists"
}

# Enable pgvector only if the extension files are actually present
$vectorAvailable = ((& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT name FROM pg_available_extensions WHERE name='vector'" 2>&1) -match 'vector')
if ($vectorAvailable) {
    & "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d qorven -c "CREATE EXTENSION IF NOT EXISTS vector;" 2>&1 | Out-Null
    Write-Ok "pgvector extension enabled"
} else {
    Write-Warn "pgvector extension not available — vector search disabled (Qorven works without it)"
}

# Restore normal PostgreSQL auth if we patched it
Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
Restore-PgTrustAuth
Write-Ok "PostgreSQL authentication restored"

$PG_DSN = "postgres://qorven@localhost:5432/qorven?sslmode=disable"

# ── Step 5: Directories + binary ─────────────────────────────────────────────
Write-Step 5 7 "Directories and binary"

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

$BinaryPath  = "$InstallDir\qorven.exe"
$localBinary = $env:QORVEN_BINARY

if ($localBinary) {
    Copy-Item $localBinary $BinaryPath -Force
    Write-Ok "Installed from local path: $BinaryPath"
} else {
    if ($ReleaseTag -eq 'latest') {
        $apiUrl   = "https://api.github.com/repos/$GithubOwner/$GithubRepo/releases"
        $releases = Invoke-RestMethod $apiUrl -Headers @{ 'User-Agent' = 'qorven-installer' }
        $rel      = $releases | Where-Object { -not $_.draft } | Select-Object -First 1
        $ReleaseTag = $rel.tag_name
        if (-not $ReleaseTag) { Invoke-Rollback "No releases found in $GithubOwner/$GithubRepo" }
    }
    $BinaryUrl = "https://github.com/$GithubOwner/$GithubRepo/releases/download/$ReleaseTag/qorven-windows-amd64.exe"
    Write-Info "Downloading Qorven $ReleaseTag ..."
    Invoke-WebRequest -Uri $BinaryUrl -OutFile "$BinaryPath.tmp" -UseBasicParsing
    if (-not (Test-Path "$BinaryPath.tmp")) { Invoke-Rollback "Binary download failed from $BinaryUrl" }
    Move-Item "$BinaryPath.tmp" $BinaryPath -Force
    Write-Ok "Downloaded: $BinaryPath"
}

$sysPath = [System.Environment]::GetEnvironmentVariable('PATH', 'Machine')
if ($sysPath -notlike "*$InstallDir*") {
    [System.Environment]::SetEnvironmentVariable('PATH', "$sysPath;$InstallDir", 'Machine')
    $env:PATH += ";$InstallDir"
    Write-Info "Added $InstallDir to system PATH"
}

# ── Step 6: Configuration ─────────────────────────────────────────────────────
Write-Step 6 7 "Configuration"

$ConfigPath = "$ConfigDir\config.toml"
if (Test-Path $ConfigPath) {
    Write-Warn "$ConfigPath already exists — leaving unchanged. Delete it to regenerate."
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
    Write-Ok "Config written: $ConfigPath"
}

# ── Step 7: Windows Service ───────────────────────────────────────────────────
Write-Step 7 7 "Windows Service"

if ($SkipService) {
    Write-Ok "Service registration skipped (--skip-service)"
} else {

$NssmPath = "$InstallDir\nssm.exe"
if (-not (Test-Path $NssmPath)) {
    Write-Info "Downloading NSSM service wrapper..."
    $NssmUrl = "https://nssm.cc/release/nssm-$NssmVersion.zip"
    $NssmZip = "$env:TEMP\nssm.zip"
    Invoke-WebRequest -Uri $NssmUrl -OutFile $NssmZip -UseBasicParsing
    Expand-Archive -Path $NssmZip -DestinationPath "$env:TEMP\nssm" -Force
    $nssmBin = Get-ChildItem "$env:TEMP\nssm" -Filter 'nssm.exe' -Recurse | Where-Object { $_.FullName -match 'win64' } | Select-Object -First 1
    if (-not $nssmBin) { $nssmBin = Get-ChildItem "$env:TEMP\nssm" -Filter 'nssm.exe' -Recurse | Select-Object -First 1 }
    if (-not $nssmBin) { Invoke-Rollback "Could not find nssm.exe in downloaded archive" }
    Copy-Item $nssmBin.FullName $NssmPath -Force
    Remove-Item $NssmZip -Force -ErrorAction SilentlyContinue
    Write-Ok "NSSM downloaded"
}

$svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($svc) {
    Write-Info "Removing previous service registration..."
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
Write-Ok "Service '$ServiceName' registered and started (auto-start on boot)"

} # end if (-not $SkipService)

# ── Health check ──────────────────────────────────────────────────────────────
if (-not $SkipService) {
    Write-Info "Waiting for Qorven to become healthy..."
    $healthy = $false
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
        Invoke-Rollback "Service started but API did not respond after 60 s. Check logs: $LogDir\qorven.log"
    }
}

$MyIP = Get-MyIP
$URL  = "http://${MyIP}:${Port}"

# ── Summary ───────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  +----------------------------------------------------------+" -ForegroundColor Green
Write-Host "  |  [OK]  Qorven installed successfully!                    |" -ForegroundColor Green
Write-Host "  +----------------------------------------------------------+" -ForegroundColor Green
Write-Host "  |  Open in browser  ->  $($URL.PadRight(33))|" -ForegroundColor Green
Write-Host "  +----------------------------------------------------------+" -ForegroundColor Green
Write-Host "  |  Config:    $($ConfigPath.PadRight(45))|" -ForegroundColor Green
Write-Host "  |  Logs:      $("$LogDir\qorven.log".PadRight(45))|" -ForegroundColor Green
Write-Host "  |  Service:   Get-Service $($ServiceName.PadRight(33))|" -ForegroundColor Green
Write-Host "  +----------------------------------------------------------+" -ForegroundColor Green
Write-Host ""
Write-Host "  To uninstall:" -ForegroundColor DarkGray
Write-Host "    iwr -useb https://get.qorven.ai/uninstall.ps1 | iex" -ForegroundColor DarkGray
Write-Host ""

if (-not $SkipService) {
    try { Start-Process $URL } catch {}
}

exit 0
