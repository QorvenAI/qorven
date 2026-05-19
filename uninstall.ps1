# Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
#
# Qorven uninstaller for Windows — removes everything install.ps1 created.
#
#   iwr -useb https://get.qorven.ai/uninstall.ps1 | iex
#
# Safe to run even if installation was partial or failed mid-way.
# Does NOT uninstall PostgreSQL itself (it may be used by other software).

#Requires -RunAsAdministrator
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Continue'   # keep going on errors — best-effort removal

if ($env:QORVEN_INSTALL_DIR) { $InstallDir = $env:QORVEN_INSTALL_DIR } else { $InstallDir = 'C:\Program Files\Qorven' }
if ($env:QORVEN_CONFIG_DIR)  { $ConfigDir  = $env:QORVEN_CONFIG_DIR }  else { $ConfigDir  = 'C:\ProgramData\Qorven' }
if ($env:PG_VERSION)         { $PgVersion  = $env:PG_VERSION }         else { $PgVersion  = '16' }

$ServiceName = 'QorvenAI'

function Write-Ok   { param($msg) Write-Host "  [OK] $msg" -ForegroundColor Green }
function Write-Skip { param($msg) Write-Host "  [--] $msg" -ForegroundColor DarkGray }
function Write-Warn { param($msg) Write-Host "  [!!] $msg" -ForegroundColor Yellow }

try { Clear-Host } catch {}
Write-Host ""
Write-Host "  Qorven Uninstaller" -ForegroundColor Red
Write-Host "  ==================" -ForegroundColor Red
Write-Host ""
Write-Host "  This will remove:" -ForegroundColor White
Write-Host "    * QorvenAI Windows service" -ForegroundColor DarkGray
Write-Host "    * Qorven binary ($InstallDir)" -ForegroundColor DarkGray
Write-Host "    * Config and data ($ConfigDir)" -ForegroundColor DarkGray
Write-Host "    * qorven database and role in PostgreSQL" -ForegroundColor DarkGray
Write-Host "    * NSSM service wrapper (if installed by Qorven)" -ForegroundColor DarkGray
Write-Host ""
Write-Host "  PostgreSQL itself is NOT removed." -ForegroundColor Yellow
Write-Host ""

$answer = Read-Host "  Continue? [y/N]"
if ($answer -notmatch '^[Yy]') { Write-Host "  Cancelled."; exit 0 }

# ── 1. Stop and remove Windows service ───────────────────────────────────────
Write-Host "`n  [1/5] Stopping service..." -ForegroundColor Cyan

$NssmPath = "$InstallDir\nssm.exe"
$svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($svc) {
    if (Test-Path $NssmPath) {
        & $NssmPath stop $ServiceName 2>&1 | Out-Null
        Start-Sleep -Seconds 2
        & $NssmPath remove $ServiceName confirm 2>&1 | Out-Null
        Write-Ok "Service '$ServiceName' stopped and removed via NSSM"
    } else {
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 2
        sc.exe delete $ServiceName 2>&1 | Out-Null
        Write-Ok "Service '$ServiceName' stopped and removed via sc.exe"
    }
} else {
    Write-Skip "Service '$ServiceName' not found"
}

# Kill any lingering qorven.exe process
$proc = Get-Process -Name 'qorven' -ErrorAction SilentlyContinue
if ($proc) {
    $proc | Stop-Process -Force
    Write-Ok "Killed running qorven.exe process"
}

# ── 2. Drop PostgreSQL database and role ─────────────────────────────────────
Write-Host "`n  [2/5] Removing PostgreSQL database..." -ForegroundColor Cyan

$PgBinDir = "C:\Program Files\PostgreSQL\$PgVersion\bin"
if (-not (Test-Path "$PgBinDir\psql.exe")) {
    $found = Get-ChildItem 'C:\Program Files\PostgreSQL' -Filter 'psql.exe' -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($found) { $PgBinDir = $found.DirectoryName }
}

$pgSvcObj = Get-Service -Name "postgresql-x64-$PgVersion" -ErrorAction SilentlyContinue
$pgSvcRunning = $pgSvcObj -and $pgSvcObj.Status -eq 'Running'

if ((Test-Path "$PgBinDir\psql.exe") -and $pgSvcRunning) {
    # Try pg_hba trust auth first (works if user set it up that way), then ask for password
    $env:PGPASSWORD = ''
    $testConn = & "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT 1" 2>&1
    if ($testConn -notmatch '1') {
        Write-Host ""
        Write-Host "  Enter the PostgreSQL 'postgres' password to drop the qorven database." -ForegroundColor Cyan
        Write-Host "  (Press Enter to skip if you don't know it)" -ForegroundColor DarkGray
        $secPw = Read-Host "  postgres password" -AsSecureString
        $bstr  = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($secPw)
        $env:PGPASSWORD = [System.Runtime.InteropServices.Marshal]::PtrToStringAuto($bstr)
        [System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
    }

    $canConnect = (& "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -tAc "SELECT 1" 2>&1) -match '1'
    if ($canConnect) {
        & "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -c "DROP DATABASE IF EXISTS qorven;" 2>&1 | Out-Null
        Write-Ok "Database 'qorven' dropped"
        & "$PgBinDir\psql.exe" -U postgres -h 127.0.0.1 -d postgres -c "DROP ROLE IF EXISTS qorven;" 2>&1 | Out-Null
        Write-Ok "Role 'qorven' dropped"
    } else {
        Write-Warn "Could not connect to PostgreSQL — database and role NOT removed. Drop manually:"
        Write-Host "    psql -U postgres -h 127.0.0.1 -c `"DROP DATABASE IF EXISTS qorven;`"" -ForegroundColor DarkGray
        Write-Host "    psql -U postgres -h 127.0.0.1 -c `"DROP ROLE IF EXISTS qorven;`"" -ForegroundColor DarkGray
    }
    Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
} else {
    Write-Skip "PostgreSQL not running or psql not found — skipping database removal"
}

# ── 3. Remove install + config directories ───────────────────────────────────
Write-Host "`n  [3/5] Removing files..." -ForegroundColor Cyan

if (Test-Path $InstallDir) {
    Remove-Item $InstallDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Ok "Removed $InstallDir"
} else {
    Write-Skip "$InstallDir not found"
}

if (Test-Path $ConfigDir) {
    Remove-Item $ConfigDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Ok "Removed $ConfigDir"
} else {
    Write-Skip "$ConfigDir not found"
}

# ── 4. Remove from system PATH ────────────────────────────────────────────────
Write-Host "`n  [4/5] Cleaning PATH..." -ForegroundColor Cyan

$sysPath = [System.Environment]::GetEnvironmentVariable('PATH', 'Machine')
if ($sysPath -like "*$InstallDir*") {
    $newPath = ($sysPath -split ';' | Where-Object { $_ -ne $InstallDir }) -join ';'
    [System.Environment]::SetEnvironmentVariable('PATH', $newPath, 'Machine')
    Write-Ok "Removed $InstallDir from system PATH"
} else {
    Write-Skip "$InstallDir was not in system PATH"
}

# ── 5. Clean up pgvector build temp files ────────────────────────────────────
Write-Host "`n  [5/5] Cleaning temp files..." -ForegroundColor Cyan

$pgvectorDir = "$env:TEMP\pgvector"
if (Test-Path $pgvectorDir) {
    Remove-Item $pgvectorDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Ok "Removed $pgvectorDir"
} else {
    Write-Skip "pgvector temp dir not found"
}

# ── Done ──────────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  +----------------------------------------------------------+" -ForegroundColor Green
Write-Host "  |  Qorven has been uninstalled.                            |" -ForegroundColor Green
Write-Host "  |  PostgreSQL is still installed and was not touched.      |" -ForegroundColor Green
Write-Host "  |  To reinstall: iwr -useb https://get.qorven.ai | iex    |" -ForegroundColor Green
Write-Host "  +----------------------------------------------------------+" -ForegroundColor Green
Write-Host ""
