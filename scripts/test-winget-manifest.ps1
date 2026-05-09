<# 
.SYNOPSIS
Validates and test-installs the DollarLint WinGet manifest on Windows.

.DESCRIPTION
By default this downloads the manifest from the dollarlint/winget-pkgs
PR branch for v0.1.3, runs winget validation, enables local manifest files,
installs the package from the manifest, verifies the dollarlint command, and
optionally uninstalls it.

Run from an elevated PowerShell prompt if LocalManifestFiles is not already
enabled. The install itself is the same local-manifest check requested by the
microsoft/winget-pkgs PR template.

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

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed with exit code ${LASTEXITCODE}: $FilePath $($Arguments -join ' ')"
    }
}

function Get-ManifestPathFromBranch {
    param(
        [string]$Owner,
        [string]$Repo,
        [string]$Branch,
        [string]$PackageVersion
    )

    $tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("dollarlint-winget-" + [guid]::NewGuid())
    $zipPath = Join-Path $tempRoot "winget-pkgs.zip"
    $extractPath = Join-Path $tempRoot "extract"
    New-Item -ItemType Directory -Path $tempRoot, $extractPath | Out-Null

    $zipUrl = "https://codeload.github.com/$Owner/$Repo/zip/refs/heads/$Branch"
    Write-Step "Downloading manifest branch"
    Write-Host $zipUrl
    Invoke-WebRequest -Uri $zipUrl -OutFile $zipPath

    Write-Step "Extracting manifest branch"
    Expand-Archive -Path $zipPath -DestinationPath $extractPath

    $manifestPath = Get-ChildItem -Path $extractPath -Recurse -Directory |
        Where-Object { $_.FullName -like "*\manifests\d\DollarLint\DollarLint\$PackageVersion" } |
        Select-Object -First 1 -ExpandProperty FullName

    if (-not $manifestPath) {
        throw "Could not find DollarLint manifest version $PackageVersion in downloaded branch."
    }

    return $manifestPath
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

Write-Step "Enabling local manifest installs"
if (-not (Test-IsAdministrator)) {
    Write-Warning "This may require an elevated PowerShell prompt. If it fails, rerun this script as Administrator."
}
Invoke-Checked winget settings --enable LocalManifestFiles

Write-Step "Installing DollarLint from local manifest"
Invoke-Checked winget install `
    --manifest $ManifestPath `
    --id $PackageIdentifier `
    --accept-source-agreements `
    --accept-package-agreements `
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
