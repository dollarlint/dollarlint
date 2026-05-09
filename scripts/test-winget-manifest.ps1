<# 
.SYNOPSIS
Validates and test-installs the DollarLint WinGet manifest on Windows.

.DESCRIPTION
By default this downloads the manifest from the dollarlint/winget-pkgs
PR branch for v0.1.3, runs winget validation, enables local manifest files,
installs the package from the manifest, verifies the dollarlint command, and
optionally uninstalls it.

Run from an elevated (Administrator) PowerShell prompt. The script enables
the LocalManifestFiles and LocalArchiveMalwareScanOverride admin settings
required for local-manifest installs.

.EXAMPLE
.\scripts\test-winget-manifest.ps1

.EXAMPLE
.\scripts\test-winget-manifest.ps1 -UninstallAfter

.EXAMPLE
.\scripts\test-winget-manifest.ps1 -ManifestPath C:\src\winget-pkgs\manifests\d\DollarLint\DollarLint\0.1.3
#>

[CmdletBinding()]
param(
    [string]$ManifestPath,
    [string]$Owner = "dollarlint",
    [string]$Repo = "winget-pkgs",
    [string]$Branch = "dollarlint-0.1.3",
    [string]$PackageIdentifier = "DollarLint.DollarLint",
    [string]$PackageVersion = "0.1.3",
    [switch]$UninstallAfter,
    [switch]$SkipCommandCheck
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Test-IsAdministrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

$WinGetErrors = @{
    "8A150001" = "WinGet internal error"
    "8A150002" = "Invalid command line arguments"
    "8A15002B" = "Portable package already exists"
    "8A15002F" = "Portable install failed"
    "8A150035" = "Archive malware scan failed - rerun as admin or pass --ignore-local-archive-malware-scan"
}

function Get-ExitCodeHex {
    param([int]$ExitCode)
    return "{0:X8}" -f ($ExitCode -band 0xFFFFFFFF)
}

function Get-LatestWinGetLog {
    $logDir = Join-Path $env:LOCALAPPDATA "Packages\Microsoft.DesktopAppInstaller_8wekyb3d8bbwe\LocalState\DiagOutputDir"
    if (-not (Test-Path $logDir)) {
        return $null
    }

    return Get-ChildItem -Path $logDir -Filter "WinGet-*.log" |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1 -ExpandProperty FullName
}

function Get-LastLogLine {
    param([string]$Path)

    if (-not $Path -or -not (Test-Path $Path)) {
        return $null
    }

    return Get-Content -Path $Path |
        Where-Object { $_ -match "\S" } |
        Select-Object -Last 1
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        $hexCode = Get-ExitCodeHex $LASTEXITCODE
        $hint = $WinGetErrors[$hexCode]
        $detail = if ($hint) { "$hint (0x$hexCode)" } else { "exit code $LASTEXITCODE (0x$hexCode)" }

        Write-Host ""
        Write-Host "Command failed: $detail" -ForegroundColor Red
        Write-Host "Command: $FilePath $($Arguments -join ' ')"

        $latestLog = $null
        if ($FilePath -eq "winget") {
            $latestLog = Get-LatestWinGetLog
            if ($latestLog) {
                Write-Host "Latest WinGet log: $latestLog"

                $lastLogLine = Get-LastLogLine $latestLog
                if ($lastLogLine) {
                    Write-Host "Last WinGet log line: $lastLogLine"

                    if ($lastLogLine -like "*IAttachmentExecute*") {
                        Write-Host "Hint: WinGet failed while applying Mark-of-the-Web with IAttachmentExecute after the installer hash was verified. This points to a Desktop App Installer/SmartScreen failure before archive extraction, not a manifest validation failure." -ForegroundColor Yellow
                    }
                }
            }
        }

        throw "Command failed: $detail"
    }
}

function Get-ManifestPathFromBranch {
    param(
        [string]$Owner,
        [string]$Repo,
        [string]$Branch,
        [string]$PackageVersion
    )

    $savedProgressPref = $ProgressPreference
    $ProgressPreference = "SilentlyContinue"

    try {
        $manifestDir = "manifests/d/DollarLint/DollarLint/$PackageVersion"
        $apiUrl = "https://api.github.com/repos/$Owner/$Repo/contents/$manifestDir`?ref=$Branch"

        Write-Step "Fetching manifest file list"
        Write-Host $apiUrl
        $files = Invoke-RestMethod -Uri $apiUrl -Headers @{ Accept = "application/vnd.github.v3+json" }

        if (-not $files -or $files.Count -eq 0) {
            throw "No manifest files found at $manifestDir on branch $Branch."
        }

        $tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("dollarlint-winget-" + [guid]::NewGuid())
        $manifestPath = Join-Path $tempRoot $manifestDir
        New-Item -ItemType Directory -Path $manifestPath -Force | Out-Null

        Write-Step "Downloading manifest files"
        foreach ($file in $files) {
            if ($file.type -ne "file") { continue }
            $outFile = Join-Path $manifestPath $file.name
            Write-Host "  $($file.name)"
            Invoke-WebRequest -Uri $file.download_url -OutFile $outFile
        }

        return $manifestPath
    }
    finally {
        $ProgressPreference = $savedProgressPref
    }
}

Write-Step "Checking winget availability"
$winget = Get-Command winget -ErrorAction Stop
Write-Host "winget: $($winget.Source)"
Invoke-Checked winget --version

if (-not $ManifestPath) {
    $ManifestPath = Get-ManifestPathFromBranch `
        -Owner $Owner `
        -Repo $Repo `
        -Branch $Branch `
        -PackageVersion $PackageVersion
}

$ManifestPath = (Resolve-Path $ManifestPath).Path
Write-Step "Using manifest"
Write-Host $ManifestPath

Write-Step "Manifest files"
Get-ChildItem -Path $ManifestPath -File | Select-Object Name, Length | Format-Table -AutoSize

Write-Step "Validating manifest"
Invoke-Checked winget validate --manifest $ManifestPath

Write-Step "Enabling admin settings for local manifest installs"
if (-not (Test-IsAdministrator)) {
    throw "This script requires an elevated (Administrator) PowerShell prompt."
}
Invoke-Checked winget settings --enable LocalManifestFiles
Invoke-Checked winget settings --enable LocalArchiveMalwareScanOverride

Write-Step "Installing DollarLint from local manifest"
$wingetCache = Join-Path $env:TEMP "WinGet"
if (Test-Path $wingetCache) {
    Write-Host "Clearing winget download cache"
    Remove-Item $wingetCache -Recurse -Force
}
Invoke-Checked winget install `
    --manifest $ManifestPath `
    --accept-source-agreements `
    --accept-package-agreements `
    --ignore-local-archive-malware-scan `
    --verbose-logs `
    --disable-interactivity

Write-Step "Checking installed package"
Invoke-Checked winget list --id $PackageIdentifier

if (-not $SkipCommandCheck) {
    Write-Step "Checking dollarlint command"
    $command = Get-Command dollarlint -ErrorAction SilentlyContinue
    if (-not $command) {
        $env:Path = [Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [Environment]::GetEnvironmentVariable("Path", "User")
        $command = Get-Command dollarlint -ErrorAction Stop
    }

    Write-Host "dollarlint: $($command.Source)"
    Invoke-Checked dollarlint --version
}

if ($UninstallAfter) {
    Write-Step "Uninstalling DollarLint"
    Invoke-Checked winget uninstall --id $PackageIdentifier --disable-interactivity
}

Write-Step "Done"
Write-Host "WinGet validation and install test completed successfully." -ForegroundColor Green
