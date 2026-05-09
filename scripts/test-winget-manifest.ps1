<# 
.SYNOPSIS
Validates and test-installs the DollarLint WinGet manifest on Windows.

.DESCRIPTION
By default this downloads the manifest from the dollarlint/winget-pkgs
newest dollarlint-x.y.z PR branch, runs winget validation, enables local manifest files,
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

.EXAMPLE
.\scripts\test-winget-manifest.ps1 -PackageVersion 0.1.5 -UninstallAfter

.EXAMPLE
.\scripts\test-winget-manifest.ps1 -Branch dollarlint-0.1.5 -UninstallAfter
#>

[CmdletBinding()]
param(
    [string]$ManifestPath,
    [string]$Owner = "dollarlint",
    [string]$Repo = "winget-pkgs",
    [string]$Branch,
    [string]$PackageIdentifier = "DollarLint.DollarLint",
    [string]$PackageVersion,
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
$script:FailureReported = $false

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

function Get-SemVerFromBranch {
    param([string]$Branch)

    if ($Branch -match "^dollarlint-v?(?<version>\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?)$") {
        return $Matches.version
    }

    return $null
}

function Resolve-ManifestSource {
    param(
        [string]$Owner,
        [string]$Repo,
        [string]$Branch,
        [string]$PackageVersion
    )

    if ($Branch -and -not $PackageVersion) {
        $PackageVersion = Get-SemVerFromBranch $Branch
        if (-not $PackageVersion) {
            throw "PackageVersion is required when Branch does not look like dollarlint-x.y.z."
        }
    }

    if ($PackageVersion -and -not $Branch) {
        $Branch = "dollarlint-$PackageVersion"
    }

    if ($Branch -and $PackageVersion) {
        return @{
            Branch = $Branch
            PackageVersion = $PackageVersion
        }
    }

    $savedProgressPref = $ProgressPreference
    $ProgressPreference = "SilentlyContinue"

    try {
        Write-Step "Discovering latest DollarLint WinGet branch"
        $apiUrl = "https://api.github.com/repos/$Owner/$Repo/branches?per_page=100"
        Write-Host $apiUrl

        $branches = Invoke-RestMethod -Uri $apiUrl -Headers @{ Accept = "application/vnd.github.v3+json" }
        $candidates = @()

        foreach ($candidateBranch in $branches) {
            $versionText = Get-SemVerFromBranch $candidateBranch.name
            if (-not $versionText) { continue }

            try {
                $candidates += [pscustomobject]@{
                    Branch = $candidateBranch.name
                    Version = [version]$versionText
                    VersionText = $versionText
                }
            }
            catch {
                Write-Host "Skipping branch with non-numeric version: $($candidateBranch.name)" -ForegroundColor Yellow
            }
        }

        $latest = $candidates | Sort-Object Version -Descending | Select-Object -First 1
        if (-not $latest) {
            throw "Could not find a branch named like dollarlint-x.y.z in $Owner/$Repo. Pass -Branch and -PackageVersion explicitly."
        }

        return @{
            Branch = $latest.Branch
            PackageVersion = $latest.VersionText
        }
    }
    finally {
        $ProgressPreference = $savedProgressPref
    }
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
        $script:FailureReported = $true

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

try {
Write-Step "Checking winget availability"
$winget = Get-Command winget -ErrorAction Stop
Write-Host "winget: $($winget.Source)"
Invoke-Checked winget --version

if (-not $ManifestPath) {
    $manifestSource = Resolve-ManifestSource `
        -Owner $Owner `
        -Repo $Repo `
        -Branch $Branch `
        -PackageVersion $PackageVersion

    $Branch = $manifestSource.Branch
    $PackageVersion = $manifestSource.PackageVersion

    Write-Host "Manifest branch: $Branch"
    Write-Host "Package version: $PackageVersion"

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
}
catch {
    if (-not $script:FailureReported) {
        Write-Host ""
        Write-Host "Script failed: $($_.Exception.Message)" -ForegroundColor Red
    }
    exit 1
}
