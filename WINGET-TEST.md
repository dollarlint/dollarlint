# WinGet distribution test findings

## Summary

The WinGet manifest for DollarLint validates and `dollarlint` runs correctly once installed, but `winget install --manifest` itself fails on this Windows ARM64 environment with `0x8A150001` in Desktop App Installer's Mark-of-the-Web handling. The same `Started applying motw using IAttachmentExecute` failure was first observed against `0.1.3` and reproduced against `0.1.5` and `0.1.6` without any DollarLint-side changes able to influence the outcome.

The failure resembles the MOTW behavior discussed in [microsoft/winget-cli#4046](https://github.com/microsoft/winget-cli/issues/4046), but that issue is closed and appears to cover trusted WinGet sources rather than local-manifest installs. Our local-manifest case may be a separate variant. The crash happens inside WinGet's `IAttachmentExecute` Mark-of-the-Web step, after the installer hash is verified and before WinGet extracts the ZIP. It is not specific to the DollarLint manifest, archive, or binary.

To keep the validation script useful in the meantime, `scripts/test-winget-manifest.ps1` now supports a `-AllowManualFallback` switch that reproduces WinGet's portable layout (Packages directory + Links alias + ARP registry entry) when WinGet aborts at the MOTW step. After the fallback runs, `winget list --id DollarLint.DollarLint` reports the package and `dollarlint --version` runs through the WinGet command alias.

## CI automation

`.github/workflows/release.yml` owns the normal release path in one Actions run: GoReleaser publishes the release outputs and opens the draft WinGet PR branch, then the `winget-manifest-validation` job waits for `dollarlint/winget-pkgs@dollarlint-x.y.z` and runs `scripts/test-winget-manifest.ps1 -PackageVersion x.y.z -Branch dollarlint-x.y.z -AllowManualFallback -UninstallAfter` on `windows-latest`. After validation passes, the same job replaces the WinGet PR body with a checked Microsoft-template description, adds the validation run URL, and marks the draft PR ready for review.

`.github/workflows/winget-validation.yml` is manual-only for ad hoc checks. Use `manifest-branch` for pre-Microsoft validation of a generated `dollarlint/winget-pkgs` branch, and `official-source` only when checking the package that Microsoft has already merged into the public WinGet source.

## Environment

- OS: Windows Desktop `10.0.26200.8246`
- Architecture: ARM64
- WinGet: `v1.28.240`
- Desktop App Installer package: `Microsoft.DesktopAppInstaller v1.28.240.0`
- Manifest branches tested: `dollarlint/winget-pkgs@dollarlint-0.1.3`, `…@dollarlint-0.1.5`, `…@dollarlint-0.1.6`
- Package identifier: `DollarLint.DollarLint`

## Failure signature

The `winget install --manifest …` invocation always fails after the hash check:

```text
Command failed: WinGet internal error (0x8A150001)
```

The final entry in the referenced WinGet log is consistently:

```text
[CORE] Started applying motw using IAttachmentExecute to <temp WinGet download path>
```

No extraction, portable registration, or command-alias step ever appears. The earlier `applying motw to … with zone: 3 / Finished applying motw` step succeeds; the crash is the *second* MOTW step, which calls `IAttachmentExecute`.

## Workarounds tried and ruled out

None of the following changed the outcome:

- Disabling SmartScreen via `HKCU\…\Policies\Attachments\SaveZoneInformation=1` (with elevation).
- Adding the installer host to Trusted Sites and forcing the HTTPS protocol default zone to Trusted.
- Mapping LAN-range IPs into the Intranet zone; WinGet still logged `zone: 3`.
- Serving the installer from `file://`, plain TCP loopback, `HttpListener` loopback, and a LAN IP HTTP server. WinGet's manifest schema rejects `file://` URLs across all schema versions; reachable HTTP variants still crashed in `IAttachmentExecute`.
- Rewriting the manifest to a non-nested portable EXE installer.
- Upgrading WinGet / Desktop App Installer (already on the latest released `1.28.240`).
- Running with and without `--ignore-local-archive-malware-scan` and `--disable-interactivity`.

All point to a Desktop App Installer failure inside `IAttachmentExecute` that the calling app cannot influence. Treat it as a WinGet/Desktop App Installer local-manifest issue unless future evidence points back to the DollarLint manifest, archive, or binary.

## Manual portable fallback (what `-AllowManualFallback` does)

When `scripts/test-winget-manifest.ps1 -AllowManualFallback` detects the `IAttachmentExecute` signature in the latest WinGet log, it reproduces WinGet's portable install by hand:

1. Parses `*.installer.yaml`, selects the entry matching the current architecture, downloads the ZIP, and verifies SHA256.
2. Expands the archive and copies the nested portable EXE to `%LOCALAPPDATA%\Microsoft\WinGet\Packages\<PackageIdentifier>_Microsoft.Winget.Source_8wekyb3d8bbwe\`.
3. Creates a hardlink (or plain copy when symlinks aren't permitted) at `%LOCALAPPDATA%\Microsoft\WinGet\Links\<command alias>.exe`.
4. Writes an ARP key at `HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\<PackageIdentifier>_Microsoft.Winget.Source_8wekyb3d8bbwe` with the same fields WinGet would write (`WinGetPackageIdentifier`, `WinGetSourceIdentifier`, `WinGetInstallerType=portable`, `DisplayName`, `DisplayVersion`, `Publisher`, `InstallDate`, `InstallLocation`, `InstallDirectoryAddedToPath`, `PortableTargetFullPath`, `PortableSymlinkFullPath`, `SHA256`).
5. Prepends the Links directory to `$env:Path` for the current process so the next `Get-Command dollarlint` succeeds.

After the fallback, the script verifies that:

- `winget list --id DollarLint.DollarLint` reports the installed version (read from the ARP entry).
- `dollarlint --version` runs from the Links alias.

When `-UninstallAfter` is set together with `-AllowManualFallback`, the script removes the Packages directory, the Links alias, and the ARP key.

## Verified result

With `scripts/test-winget-manifest.ps1 -AllowManualFallback -UninstallAfter` against the `0.1.6` manifest:

- `winget validate --manifest …` → `Manifest validation succeeded.`
- WinGet selects the correct ARM64 ZIP and verifies its hash.
- `winget install` fails at `IAttachmentExecute` (expected on the ARM64 environment where this local-manifest failure reproduces).
- Fallback installs the portable EXE, links it, and registers ARP.
- `winget list --id DollarLint.DollarLint` reports `DollarLint 0.1.6`.
- `dollarlint --version` prints `dollarlint version 0.1.6`.
- Uninstall removes all three artifacts and the package no longer appears in `winget list`.

## Script changes

`scripts/test-winget-manifest.ps1` now:

- Auto-detects whether `LocalManifestFiles` and `LocalArchiveMalwareScanOverride` are already enabled and skips the elevation check when they are. Elevation is only required to flip them on the first time.
- Catches the `winget install` failure, surfaces the `IAttachmentExecute` hint, and offers `-AllowManualFallback` in the message.
- When `-AllowManualFallback` is set and the failure is the known MOTW crash, runs the manual portable install described above.
- Distinguishes the "Done" message and `-UninstallAfter` branch depending on whether WinGet or the manual fallback performed the install.

## Recommended next steps

1. Re-run `scripts/test-winget-manifest.ps1 -UninstallAfter` (without the fallback) on future Desktop App Installer releases to check whether `winget install --manifest` completes natively. If it still fails, consider filing a new `microsoft/winget-cli` issue specifically for local-manifest installs.
2. Re-run the script on an x64 Windows host to confirm whether the crash is ARM64-specific or affects all architectures on `1.28.240`.
3. Treat the manifest as syntactically valid. Until the local-manifest MOTW failure is fixed or explained, use `-AllowManualFallback` to confirm the portable layout end-to-end on affected machines; without that switch, the install step will continue to fail at `IAttachmentExecute` there.
