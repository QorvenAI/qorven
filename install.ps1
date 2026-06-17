# Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
#
# Qorven installer for Windows — one-shot PowerShell script.
#
#   iwr -useb https://get.qorven.ai | iex
#   iwr -useb https://get.qorven.ai/install.ps1 | iex
#
# What it does:
#   1. Installs PostgreSQL (winget-first; EDB MSI fallback if winget is absent or fails)
#   2. Installs pgvector extension (required — the schema uses vector columns)
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
# How (if at all) THIS run installed PostgreSQL, so rollback uninstalls it with
# the right tool and NEVER removes a PostgreSQL the user already had.
#   'none'   — PG pre-existed (or we didn't install it): do NOT touch it.
#   'winget' — we installed it via winget: roll back with winget uninstall.
#   'msi'    — we installed it via the EDB MSI: roll back with the MSI uninstaller.
$script:RollbackPgMethod          = 'none'
$script:RollbackCreatedRole       = $false  # we ran CREATE ROLE qorven
$script:RollbackCreatedDb         = $false  # we ran CREATE DATABASE qorven
$script:RollbackCreatedInstallDir = $false  # we created $InstallDir
$script:RollbackCreatedConfigDir  = $false  # we created $ConfigDir
$script:RollbackCreatedService    = $false  # we registered the service
$script:PgBinDir                  = ''


# ── rollback ──────────────────────────────────────────────────────────────────
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

    # PostgreSQL — only if THIS run installed it, and with the matching uninstaller.
    # Never remove a PostgreSQL that pre-existed ('none').
    if ($script:RollbackPgMethod -eq 'winget' -and (Get-Command winget -ErrorAction SilentlyContinue)) {
        Write-Rb "Removing PostgreSQL (installed via winget this run)..."
        winget uninstall --id PostgreSQL.PostgreSQL.$PgVersion --silent 2>&1 | Out-Null
        Write-Rb "PostgreSQL removed"
    } elseif ($script:RollbackPgMethod -eq 'msi') {
        # EDB MSI registers an ARP uninstall entry; invoke it silently if found.
        Write-Rb "Removing PostgreSQL (installed via EDB MSI this run)..."
        $unins = "C:\Program Files\PostgreSQL\$PgVersion\uninstall-postgresql.exe"
        if (Test-Path $unins) {
            & $unins --mode unattended 2>&1 | Out-Null
            Write-Rb "PostgreSQL removed"
        } else {
            Write-Rb "EDB uninstaller not found at $unins — remove PostgreSQL $PgVersion manually if unwanted"
        }
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

# 32-bit check — we only ship amd64
if ([System.Environment]::Is64BitOperatingSystem -eq $false -or [System.Environment]::Is64BitProcess -eq $false) {
    Invoke-Rollback "Qorven requires a 64-bit Windows installation. 32-bit is not supported."
}

# OS version floor — require Windows 10 / Server 2019 (NT 10.0) or later
$osVer = [System.Environment]::OSVersion.Version
if ($osVer.Major -lt 10) {
    Invoke-Rollback "Qorven requires Windows 10 / Windows Server 2019 or later (detected: Windows $($osVer.Major).$($osVer.Minor))."
}

$WingetAvail = Command-Exists 'winget'
if ($WingetAvail) {
    Write-Ok "winget found: $(winget --version)"
} else {
    Write-Warn "winget not found — will rely on pre-installed software"
}

# Detect any existing PostgreSQL service (any version) and auto-detect $PgVersion
$existingPgService = Get-Service -Name "postgresql-x64-*" -ErrorAction SilentlyContinue | Select-Object -First 1
if ($existingPgService) {
    $detectedVersion = ($existingPgService.Name -replace 'postgresql-x64-', '')
    if ($detectedVersion -ne $PgVersion) {
        Write-Warn "Detected PostgreSQL $detectedVersion (expected $PgVersion) — using detected version"
        $PgVersion = $detectedVersion
    }
}

# Pre-check: port 5432 availability (only relevant if we need to install PG fresh)
if (-not $existingPgService) {
    $portInUse = $false
    try {
        $tcp = [System.Net.Sockets.TcpClient]::new()
        $tcp.Connect('127.0.0.1', 5432)
        $tcp.Close()
        $portInUse = $true
    } catch {}
    if ($portInUse) {
        Invoke-Rollback "Port 5432 is already in use by another process. Identify and stop it before installing PostgreSQL:`n  netstat -ano | findstr :5432"
    }
}

Write-Ok "Prerequisites OK"

# ── Step 2: PostgreSQL ────────────────────────────────────────────────────────
Write-Step 2 7 "PostgreSQL"

# Known superuser password — used by the EDB MSI path (--superpassword flag).
# The winget path may override this if winget PG uses passwordless/trust auth.
# Either way we never prompt interactively.
$PgSuperPass = Random-Hex 16
$env:PGPASSWORD = $PgSuperPass

$pgService = Get-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
if ($pgService) {
    Write-Ok "PostgreSQL $PgVersion service already present"

    # Resolve psql.exe path FIRST before any password attempt
    $PgBinDir = "C:\Program Files\PostgreSQL\$PgVersion\bin"
    if (-not (Test-Path "$PgBinDir\psql.exe")) {
        $found = Get-ChildItem 'C:\Program Files\PostgreSQL' -Filter 'psql.exe' -Recurse -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -like "*$PgVersion*" } | Select-Object -First 1
        if (-not $found) {
            $found = Get-ChildItem 'C:\Program Files\PostgreSQL' -Filter 'psql.exe' -Recurse -ErrorAction SilentlyContinue |
                Select-Object -First 1
        }
        if ($found) { $PgBinDir = $found.DirectoryName }
        else { Invoke-Rollback "psql.exe not found for PostgreSQL $PgVersion — is it installed correctly?" }
    }
    $script:PgBinDir = $PgBinDir
    $env:PATH += ";$PgBinDir"

    # Try passwordless first (peer/trust auth or Windows authentication)
    $env:PGPASSWORD = ''
    $canTest = ((& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT 1" 2>&1) -match '1')
    if ($canTest) {
        Write-Ok "Connected to existing PostgreSQL (no password needed)"
    } elseif ($env:PG_SUPERUSER_PASSWORD) {
        $env:PGPASSWORD = $env:PG_SUPERUSER_PASSWORD
        $canTest = ((& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT 1" 2>&1) -match '1')
        if (-not $canTest) { Invoke-Rollback "PG_SUPERUSER_PASSWORD env var set but connection failed" }
    } else {
        # No passwordless connection and PG_SUPERUSER_PASSWORD is not set.
        # Unattended installs must not block on interactive prompts.
        Invoke-Rollback (
            "Existing PostgreSQL requires a password and none was supplied.`n" +
            "  Re-run with the superuser password in the environment:`n" +
            "    `$env:PG_SUPERUSER_PASSWORD='your-password'; iwr -useb https://get.qorven.ai/install.ps1 | iex`n" +
            "  Or use --skip-postgres and supply a DSN via QORVEN_POSTGRES_DSN to point at an existing database."
        )
    }
} else {
    # Fresh install — try winget first (resolves the current release automatically,
    # no brittle URL or hardcoded patch-suffix). Fall back to EDB MSI when winget
    # is unavailable or fails.
    $pgInstalledViaWinget = $false

    if ($WingetAvail) {
        Write-Info "Installing PostgreSQL $PgVersion via winget..."
        $wingetResult = winget install --id "PostgreSQL.PostgreSQL.$PgVersion" `
            --silent --accept-package-agreements --accept-source-agreements 2>&1
        $wgAlready = ($wingetResult -match 'already installed')
        if ($LASTEXITCODE -eq 0 -or $wingetResult -match 'successfully installed' -or $wgAlready) {
            $pgInstalledViaWinget = $true
            # Only mark for rollback if WE actually installed it this run. If winget
            # reports "already installed", PostgreSQL pre-existed — never uninstall it.
            if ($wgAlready) {
                $script:RollbackPgMethod = 'none'
                Write-Ok "PostgreSQL $PgVersion already present (winget) — reusing it"
            } else {
                $script:RollbackPgMethod = 'winget'
                Write-Ok "PostgreSQL $PgVersion installed via winget"
            }
            Start-Sleep -Seconds 5

            # winget's PostgreSQL package does not set a known superuser password
            # (it uses Windows authentication / trust for the local postgres account).
            # Attempt a passwordless connection; if that works we're done.  If not,
            # check PG_SUPERUSER_PASSWORD.  winget PG installs with the postgres
            # Windows service account — local connections via 127.0.0.1 may require
            # a password set by the user during install (depends on winget package version).
            #
            # Strategy: find psql first, then probe.
            $PgBinDirTmp = "C:\Program Files\PostgreSQL\$PgVersion\bin"
            if (-not (Test-Path "$PgBinDirTmp\psql.exe")) {
                $found = Get-ChildItem 'C:\Program Files\PostgreSQL' -Filter 'psql.exe' -Recurse -ErrorAction SilentlyContinue |
                    Select-Object -First 1
                if ($found) { $PgBinDirTmp = $found.DirectoryName }
            }

            if (Test-Path "$PgBinDirTmp\psql.exe") {
                $env:PGPASSWORD = ''
                $wingetPwdless = ((& "$PgBinDirTmp\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT 1" 2>&1) -match '1')
                if (-not $wingetPwdless -and $env:PG_SUPERUSER_PASSWORD) {
                    $env:PGPASSWORD = $env:PG_SUPERUSER_PASSWORD
                    $wingetPwdless = ((& "$PgBinDirTmp\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT 1" 2>&1) -match '1')
                }
                if ($wingetPwdless) {
                    # We have a working connection — set $PgSuperPass to the env var value
                    # (or empty for passwordless) so Step 4 can connect.
                    if ($env:PG_SUPERUSER_PASSWORD) {
                        $PgSuperPass = $env:PG_SUPERUSER_PASSWORD
                        $env:PGPASSWORD = $PgSuperPass
                    }
                    # else: $env:PGPASSWORD stays '' (passwordless)
                } else {
                    # winget PG installed but can't connect — fall through to EDB MSI which sets a known password.
                    Write-Warn "winget PostgreSQL installed but connection test failed — falling back to EDB MSI for a known-password install."
                    winget uninstall --id "PostgreSQL.PostgreSQL.$PgVersion" --silent 2>&1 | Out-Null
                    $pgInstalledViaWinget = $false
                    $script:RollbackPgMethod = 'none'
                    $env:PGPASSWORD = $PgSuperPass  # restore fresh random password for MSI path
                }
            } else {
                # psql not found after winget install — fall back to EDB MSI
                Write-Warn "psql.exe not found after winget install — falling back to EDB MSI."
                winget uninstall --id "PostgreSQL.PostgreSQL.$PgVersion" --silent 2>&1 | Out-Null
                $pgInstalledViaWinget = $false
                $script:RollbackPgMethod = 'none'
                $env:PGPASSWORD = $PgSuperPass
            }
        } else {
            Write-Warn "winget install failed (exit $LASTEXITCODE) — falling back to EDB MSI."
            $env:PGPASSWORD = $PgSuperPass
        }
    }

    if (-not $pgInstalledViaWinget) {
        # EDB MSI path — the MSI accepts --superpassword so we set a known password.
        # EDB's URL uses a patch-release suffix (e.g. -1-, -2-) that changes with each
        # minor update.  Try suffixes 1–4 before giving up.
        $MsiPath = "$env:TEMP\pg-installer.exe"
        $downloaded = $false
        Write-Info "Downloading PostgreSQL $PgVersion installer from EDB (~250 MB)..."
        foreach ($patchSuffix in @(1, 2, 3, 4)) {
            $MsiUrl = "https://get.enterprisedb.com/postgresql/postgresql-$PgVersion-$patchSuffix-windows-x64.exe"
            try {
                Invoke-WebRequest -Uri $MsiUrl -OutFile $MsiPath -UseBasicParsing -ErrorAction Stop
                $downloaded = $true
                Write-Info "Downloaded: postgresql-$PgVersion-$patchSuffix-windows-x64.exe"
                break
            } catch {
                Write-Info "Suffix -$patchSuffix not found, trying next..."
                Remove-Item $MsiPath -Force -ErrorAction SilentlyContinue
            }
        }
        if (-not $downloaded) {
            Invoke-Rollback "PostgreSQL installer download failed (tried patch suffixes 1-4 for version $PgVersion). Download manually from https://www.postgresql.org/download/windows/ and re-run."
        }
        if (-not (Test-Path $MsiPath)) { Invoke-Rollback "PostgreSQL installer not found after download" }

        Write-Info "Installing PostgreSQL $PgVersion (unattended)..."
        $installArgs = @(
            '--mode', 'unattended',
            '--superpassword', $PgSuperPass,
            '--servicename', "postgresql-x64-$PgVersion",
            '--servicepassword', 'NT AUTHORITY\NetworkService',
            '--datadir', "C:\Program Files\PostgreSQL\$PgVersion\data",
            '--serverport', '5432',
            '--unattendedmodeui', 'none'
        )
        $proc = Start-Process -FilePath $MsiPath -ArgumentList $installArgs -Wait -PassThru
        Remove-Item $MsiPath -Force -ErrorAction SilentlyContinue
        if ($proc.ExitCode -ne 0) { Invoke-Rollback "PostgreSQL installer exited with code $($proc.ExitCode)" }
        $script:RollbackPgMethod = 'msi'
        Write-Ok "PostgreSQL $PgVersion installed (EDB MSI)"
        Start-Sleep -Seconds 5
    }
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

# ── Step 3: pgvector (required) ───────────────────────────────────────────────
Write-Step 3 7 "pgvector (vector search — required)"

# Download and install a pre-built pgvector ZIP from the pgvector GitHub releases.
# No Visual Studio Build Tools required — pure binary install.
$pgvectorInstalled = $false
$pgvectorZip = "$env:TEMP\pgvector.zip"
$pgvectorTmp = "$env:TEMP\pgvector-install"
$pgDataDir   = "C:\Program Files\PostgreSQL\$PgVersion"

try {
    # Resolve latest pgvector release tag
    $pgvRelease = Invoke-RestMethod 'https://api.github.com/repos/pgvector/pgvector/releases/latest' `
        -Headers @{ 'User-Agent' = 'qorven-installer' } -ErrorAction Stop
    $pgvTag = $pgvRelease.tag_name  # e.g. v0.7.4

    # Binaries for Windows are published as pgvector-{tag}-pg{major}-windows-x86_64.zip
    $pgvFile = "pgvector-$pgvTag-pg${PgVersion}-windows-x86_64.zip"
    $pgvUrl  = "https://github.com/pgvector/pgvector/releases/download/$pgvTag/$pgvFile"

    Write-Info "Downloading pgvector $pgvTag..."
    Invoke-WebRequest -Uri $pgvUrl -OutFile $pgvectorZip -UseBasicParsing -ErrorAction Stop

    Expand-Archive -Path $pgvectorZip -DestinationPath $pgvectorTmp -Force -ErrorAction Stop

    # Copy lib/*.dll → PostgreSQL lib dir, share/extension/* → extension dir
    $src = Get-ChildItem $pgvectorTmp -Recurse -Directory | Select-Object -First 1
    if (-not $src) { $src = [System.IO.DirectoryInfo]$pgvectorTmp }
    $libSrc = Join-Path $src.FullName 'lib'
    $extSrc = Join-Path $src.FullName 'share\extension'
    if (Test-Path $libSrc) {
        Copy-Item "$libSrc\*" "$pgDataDir\lib\" -Recurse -Force
    }
    if (Test-Path $extSrc) {
        New-Item -ItemType Directory -Force -Path "$pgDataDir\share\extension" | Out-Null
        Copy-Item "$extSrc\*" "$pgDataDir\share\extension\" -Recurse -Force
    }

    $pgvectorInstalled = $true
    Write-Ok "pgvector $pgvTag installed (pre-built binary)"
} catch {
    Write-Warn "pgvector download/install failed here — will verify and retry at database setup."
    Write-Info "Manual install reference: https://github.com/pgvector/pgvector#windows"
} finally {
    Remove-Item $pgvectorZip  -Force -ErrorAction SilentlyContinue
    Remove-Item $pgvectorTmp  -Recurse -Force -ErrorAction SilentlyContinue
}

# ── Step 4: Database setup ────────────────────────────────────────────────────
Write-Step 4 7 "Database setup"

# $env:PGPASSWORD was set in Step 2 — the random password from the EDB MSI install,
# the PG_SUPERUSER_PASSWORD env var for a pre-existing installation, or '' for
# passwordless/trust auth. No pg_hba.conf patching required.
$canConnect = ((& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT 1" 2>&1) -match '1')
if (-not $canConnect) {
    Invoke-Rollback "Cannot connect to PostgreSQL. Check the service is running and the password is correct."
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

# pgvector is a HARD requirement, NOT optional: the schema declares vector(384)/
# vector(1536) columns and ivfflat/hnsw indexes. Without the extension the
# migration that runs at service start dies with "type vector does not exist",
# surfacing only as a generic service-start failure. Enable it here and fail
# early with a clear message if it cannot be enabled.
$vectorAvailable = ((& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT name FROM pg_available_extensions WHERE name='vector'" 2>&1) -match 'vector')
if ($vectorAvailable) {
    & "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d qorven -c "CREATE EXTENSION IF NOT EXISTS vector;" 2>&1 | Out-Null
}
$vectorEnabled = ((& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d qorven -tAc "SELECT 1 FROM pg_extension WHERE extname='vector'" 2>&1) -match '1')
if ($vectorEnabled) {
    Write-Ok "pgvector extension enabled"
} else {
    Invoke-Rollback "pgvector could not be enabled — it is REQUIRED (the schema uses vector columns). Re-run Step 3, or install the pre-built pgvector binary for PostgreSQL $PgVersion from https://github.com/pgvector/pgvector#windows, then re-run this installer."
}

Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue

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

# Preserve existing encryption_key and token on re-run — regenerating these
# would make all stored API keys and secrets permanently unreadable.
$ExistingEncKey = ''
$ExistingToken  = ''
if (Test-Path $ConfigPath) {
    $existingLines = Get-Content $ConfigPath -ErrorAction SilentlyContinue
    foreach ($line in $existingLines) {
        if ($line -match '^encryption_key\s*=\s*"(.+)"') { $ExistingEncKey = $Matches[1] }
        if ($line -match '^token\s*=\s*"(.+)"')          { $ExistingToken  = $Matches[1] }
    }
}

$EncKey    = if ($ExistingEncKey) { $ExistingEncKey } else { Random-Hex 32 }
$AuthToken = if ($ExistingToken)  { $ExistingToken  } else { Random-Hex 16 }

if ($ExistingEncKey) {
    Write-Ok "Config preserved (existing encryption_key retained)"
} else {
    $configContent = @"
# Qorven configuration — generated by install.ps1
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
$UseNssm = $false
if (-not (Test-Path $NssmPath)) {
    Write-Info "Downloading NSSM service wrapper..."
    $NssmUrl = "https://nssm.cc/release/nssm-$NssmVersion.zip"
    $NssmZip = "$env:TEMP\nssm.zip"
    try {
        Invoke-WebRequest -Uri $NssmUrl -OutFile $NssmZip -UseBasicParsing -ErrorAction Stop
        Expand-Archive -Path $NssmZip -DestinationPath "$env:TEMP\nssm" -Force
        $nssmBin = Get-ChildItem "$env:TEMP\nssm" -Filter 'nssm.exe' -Recurse |
            Where-Object { $_.FullName -match 'win64' } | Select-Object -First 1
        if (-not $nssmBin) {
            $nssmBin = Get-ChildItem "$env:TEMP\nssm" -Filter 'nssm.exe' -Recurse | Select-Object -First 1
        }
        if ($nssmBin) {
            Copy-Item $nssmBin.FullName $NssmPath -Force
            $UseNssm = $true
            Write-Ok "NSSM downloaded"
        } else {
            Write-Warn "NSSM archive did not contain nssm.exe — falling back to sc.exe"
        }
    } catch {
        Write-Warn "NSSM download failed ($_) — falling back to sc.exe for service registration"
    } finally {
        Remove-Item $NssmZip -Force -ErrorAction SilentlyContinue
        Remove-Item "$env:TEMP\nssm" -Recurse -Force -ErrorAction SilentlyContinue
    }
} else {
    $UseNssm = $true
}

$svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($svc) {
    Write-Info "Removing previous service registration..."
    if ($UseNssm) {
        & $NssmPath stop $ServiceName 2>&1 | Out-Null
        & $NssmPath remove $ServiceName confirm 2>&1 | Out-Null
    } else {
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
        sc.exe delete $ServiceName 2>&1 | Out-Null
        Start-Sleep -Seconds 2
    }
}

if ($UseNssm) {
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
} else {
    # Fallback: register using sc.exe with a wrapper batch that sets env vars
    $batchPath = "$InstallDir\qorven-start.bat"
    $batchContent = "@echo off`r`nset QORVEN_CONFIG=$ConfigPath`r`n`"$BinaryPath`" start >> `"$LogDir\qorven.log`" 2>&1"
    Set-Content -Path $batchPath -Value $batchContent -Encoding ASCII
    sc.exe create $ServiceName binPath= "`"$batchPath`"" start= auto DisplayName= "Qorven AI Platform" 2>&1 | Out-Null
    sc.exe description $ServiceName "Qorven AI Agent Platform" 2>&1 | Out-Null
    sc.exe failure $ServiceName reset= 60 actions= restart/5000/restart/10000/restart/30000 2>&1 | Out-Null
}
$script:RollbackCreatedService = $true
Start-Service -Name $ServiceName -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2
$svcState = (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue).Status
if ($svcState -ne 'Running') {
    Invoke-Rollback "Service '$ServiceName' failed to start. Check logs: $LogDir\qorven.log"
}
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
