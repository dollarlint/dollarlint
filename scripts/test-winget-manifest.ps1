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
    [switch]$SkipCommandCheck,
    [switch]$AllowManualFallback
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
    "8A150028" = "Manifest validation completed with warnings"
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

        if ($FilePath -eq "winget" -and $Arguments.Count -gt 0 -and $Arguments[0] -eq "validate" -and $hexCode -eq "8A150028") {
            Write-Host ""
            Write-Host "Command completed with accepted warning: $detail" -ForegroundColor Yellow
            return
        }

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
                        Write-Host "Hint: WinGet failed while applying Mark-of-the-Web with IAttachmentExecute after the installer hash was verified." -ForegroundColor Yellow
                        Write-Host "      This resembles the WinGet MOTW behavior discussed in microsoft/winget-cli#4046," -ForegroundColor Yellow
                        Write-Host "      but local-manifest installs may be a separate variant. The failure happens" -ForegroundColor Yellow
                        Write-Host "      after hash verification and before archive extraction, which points away from" -ForegroundColor Yellow
                        Write-Host "      the DollarLint manifest, archive layout, or binary." -ForegroundColor Yellow
                        Write-Host "      Re-run with -AllowManualFallback to skip WinGet's install and verify the portable layout manually." -ForegroundColor Yellow
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

function Get-NativeArchitecture {
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ($env:PROCESSOR_ARCHITEW6432) { $arch = $env:PROCESSOR_ARCHITEW6432 }
    switch ($arch.ToUpperInvariant()) {
        "ARM64" { return "arm64" }
        "AMD64" { return "x64" }
        "X86"   { return "x86" }
        default { return $arch.ToLowerInvariant() }
    }
}

function Get-InstallerEntryForCurrentArch {
    param([Parameter(Mandatory = $true)][string]$ManifestPath)

    $installerYaml = Get-ChildItem -Path $ManifestPath -Filter "*.installer.yaml" -File |
        Select-Object -First 1 -ExpandProperty FullName
    if (-not $installerYaml) {
        throw "Could not locate *.installer.yaml under $ManifestPath."
    }

    $lines = Get-Content -Path $installerYaml
    $entries = @()
    $current = $null
    foreach ($line in $lines) {
        if ($line -match "^\s*-\s*Architecture:\s*(?<arch>\S+)") {
            if ($current) { $entries += [pscustomobject]$current }
            $current = @{ Architecture = $Matches.arch; InstallerUrl = $null; InstallerSha256 = $null; NestedRelativeFilePath = $null }
        }
        elseif ($current) {
            if ($line -match "^\s*InstallerUrl:\s*(?<url>\S+)")              { $current.InstallerUrl = $Matches.url }
            elseif ($line -match "^\s*InstallerSha256:\s*(?<sha>\S+)")       { $current.InstallerSha256 = $Matches.sha.ToLowerInvariant() }
            elseif ($line -match "^\s*-\s*RelativeFilePath:\s*(?<path>\S+)") { $current.NestedRelativeFilePath = $Matches.path }
        }
    }
    if ($current) { $entries += [pscustomobject]$current }

    $arch = Get-NativeArchitecture
    $entry = $entries | Where-Object { $_.Architecture -ieq $arch } | Select-Object -First 1
    if (-not $entry) {
        throw "Manifest does not declare an installer for architecture '$arch'."
    }
    return $entry
}

function Invoke-ManualPortableFallback {
    param(
        [Parameter(Mandatory = $true)][string]$ManifestPath,
        [Parameter(Mandatory = $true)][string]$PackageIdentifier,
        [Parameter(Mandatory = $true)][string]$PackageVersion
    )

    $entry = Get-InstallerEntryForCurrentArch -ManifestPath $ManifestPath
    if (-not $entry.NestedRelativeFilePath) {
        throw "Manual fallback currently supports nested portable ZIP manifests only (missing RelativeFilePath)."
    }

    $sourceIdentifier = "Microsoft.Winget.Source_8wekyb3d8bbwe"
    $productCode = "$($PackageIdentifier)_$sourceIdentifier"
    $pkgRoot = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages\$productCode"
    $linksDir = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Links"
    $work = Join-Path ([System.IO.Path]::GetTempPath()) ("dollarlint-manual-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Path $pkgRoot, $linksDir, $work -Force | Out-Null

    $archive = Join-Path $work ([System.IO.Path]::GetFileName($entry.InstallerUrl))
    Write-Host "Downloading $($entry.InstallerUrl)"
    $savedProgressPref = $ProgressPreference
    $ProgressPreference = "SilentlyContinue"
    try { Invoke-WebRequest -Uri $entry.InstallerUrl -OutFile $archive }
    finally { $ProgressPreference = $savedProgressPref }

    $actualHash = (Get-FileHash -Path $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $entry.InstallerSha256) {
        throw "Installer hash mismatch. Expected $($entry.InstallerSha256) Actual $actualHash"
    }

    Expand-Archive -Path $archive -DestinationPath $work -Force
    $extracted = Join-Path $work $entry.NestedRelativeFilePath
    if (-not (Test-Path $extracted)) {
        throw "Nested installer file not found after extraction: $extracted"
    }

    $exeName = [System.IO.Path]::GetFileName($extracted)
    $target = Join-Path $pkgRoot $exeName
    Copy-Item -Path $extracted -Destination $target -Force
    Unblock-File -Path $target -ErrorAction SilentlyContinue

    $link = Join-Path $linksDir $exeName
    Remove-Item -Path $link -Force -ErrorAction SilentlyContinue
    try { New-Item -ItemType HardLink -Path $link -Target $target -ErrorAction Stop | Out-Null }
    catch { Copy-Item -Path $target -Destination $link -Force }
    Unblock-File -Path $link -ErrorAction SilentlyContinue

    $shaHex = (Get-FileHash -Path $target -Algorithm SHA256).Hash
    $shaBytes = for ($i = 0; $i -lt $shaHex.Length; $i += 2) { [Convert]::ToByte($shaHex.Substring($i, 2), 16) }

    $key = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\$productCode"
    New-Item -Path $key -Force | Out-Null
    Set-ItemProperty -Path $key -Name WinGetPackageIdentifier     -Type String -Value $PackageIdentifier
    Set-ItemProperty -Path $key -Name WinGetSourceIdentifier      -Type String -Value $sourceIdentifier
    Set-ItemProperty -Path $key -Name UninstallString             -Type String -Value "winget uninstall --product-code $productCode"
    Set-ItemProperty -Path $key -Name WinGetInstallerType         -Type String -Value "portable"
    Set-ItemProperty -Path $key -Name DisplayName                 -Type String -Value $PackageIdentifier.Split(".")[-1]
    Set-ItemProperty -Path $key -Name DisplayVersion              -Type String -Value $PackageVersion
    Set-ItemProperty -Path $key -Name Publisher                   -Type String -Value $PackageIdentifier.Split(".")[0]
    Set-ItemProperty -Path $key -Name InstallDate                 -Type String -Value (Get-Date -Format "yyyyMMdd")
    Set-ItemProperty -Path $key -Name InstallDirectoryCreated     -Type DWord  -Value 1
    Set-ItemProperty -Path $key -Name InstallLocation             -Type String -Value $pkgRoot
    Set-ItemProperty -Path $key -Name InstallDirectoryAddedToPath -Type DWord  -Value 1
    Set-ItemProperty -Path $key -Name PortableTargetFullPath      -Type String -Value $target
    Set-ItemProperty -Path $key -Name PortableSymlinkFullPath     -Type String -Value $link
    Set-ItemProperty -Path $key -Name SHA256                      -Type Binary -Value ([byte[]]$shaBytes)

    Write-Host "Installed $target"
    Write-Host "Registered ARP key $key"

    if ($env:Path -notlike "*$linksDir*") {
        $env:Path = "$linksDir;$env:Path"
    }
}

function Remove-ManualPortableInstall {
    param([Parameter(Mandatory = $true)][string]$PackageIdentifier)

    $productCode = "$($PackageIdentifier)_Microsoft.Winget.Source_8wekyb3d8bbwe"
    $pkgRoot = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages\$productCode"
    $linksDir = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Links"
    $key = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\$productCode"

    $link = $null
    if (Test-Path $key) {
        $props = Get-ItemProperty -Path $key
        $link = $props.PortableSymlinkFullPath
        Remove-Item -Path $key -Recurse -Force -ErrorAction SilentlyContinue
    }

    if ($link -and (Test-Path $link)) { Remove-Item -Path $link -Force -ErrorAction SilentlyContinue }
    if (Test-Path $pkgRoot) { Remove-Item -Path $pkgRoot -Recurse -Force -ErrorAction SilentlyContinue }
    Write-Host "Removed manual portable install for $PackageIdentifier"
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
$settingsJson = winget settings export 2>$null | Out-String
$localManifestEnabled = $settingsJson -match '"LocalManifestFiles"\s*:\s*true'
$archiveOverrideEnabled = $settingsJson -match '"LocalArchiveMalwareScanOverride"\s*:\s*true'
if ($localManifestEnabled -and $archiveOverrideEnabled) {
    Write-Host "LocalManifestFiles and LocalArchiveMalwareScanOverride already enabled; skipping elevation."
}
else {
    if (-not (Test-IsAdministrator)) {
        throw "This script requires an elevated (Administrator) PowerShell prompt to enable LocalManifestFiles / LocalArchiveMalwareScanOverride."
    }
    Invoke-Checked winget settings --enable LocalManifestFiles
    Invoke-Checked winget settings --enable LocalArchiveMalwareScanOverride
}

Write-Step "Installing DollarLint from local manifest"
$wingetCache = Join-Path $env:TEMP "WinGet"
if (Test-Path $wingetCache) {
    Write-Host "Clearing winget download cache"
    Remove-Item $wingetCache -Recurse -Force
}

$script:UsedManualFallback = $false
try {
    Invoke-Checked winget install `
        --manifest $ManifestPath `
        --accept-source-agreements `
        --accept-package-agreements `
        --ignore-local-archive-malware-scan `
        --verbose-logs `
        --disable-interactivity
}
catch {
    if (-not $AllowManualFallback) {
        throw
    }

    $latestLog = Get-LatestWinGetLog
    $lastLogLine = Get-LastLogLine $latestLog
    if ($lastLogLine -notlike "*IAttachmentExecute*") {
        throw
    }

    Write-Host ""
    Write-Step "Falling back to manual portable install (WinGet MOTW local-manifest failure)"
    Invoke-ManualPortableFallback -ManifestPath $ManifestPath -PackageIdentifier $PackageIdentifier -PackageVersion $PackageVersion
    $script:UsedManualFallback = $true
}

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
    if ($script:UsedManualFallback) {
        Remove-ManualPortableInstall -PackageIdentifier $PackageIdentifier
    }
    else {
        Invoke-Checked winget uninstall --id $PackageIdentifier --disable-interactivity
    }
}

Write-Step "Done"
if ($script:UsedManualFallback) {
    Write-Host "WinGet validation succeeded; install verified via manual portable fallback (MOTW bug workaround)." -ForegroundColor Yellow
}
else {
    Write-Host "WinGet validation and install test completed successfully." -ForegroundColor Green
}
}
catch {
    if (-not $script:FailureReported) {
        Write-Host ""
        Write-Host "Script failed: $($_.Exception.Message)" -ForegroundColor Red
    }
    exit 1
}
