# WinGet distribution test findings

## Summary

The WinGet manifest for DollarLint `0.1.3` validates successfully, but the full local-manifest install test does not currently complete on this Windows ARM64 environment.

The failure occurs after WinGet downloads the selected ARM64 ZIP and verifies its SHA256 hash. The latest WinGet logs stop at the Desktop App Installer `IAttachmentExecute` Mark-of-the-Web (MOTW) step, before archive extraction or `dollarlint.exe` execution. Based on the evidence, this looks more like a WinGet/Desktop App Installer or SmartScreen/MOTW failure than a DollarLint binary, archive, or manifest validation issue.

## Environment

- OS: Windows Desktop `10.0.26200.8246`
- Architecture: ARM64
- WinGet: `v1.28.240`
- Desktop App Installer package: `Microsoft.DesktopAppInstaller v1.28.240.0`
- Manifest branch tested: `dollarlint/winget-pkgs@dollarlint-0.1.3`
- Package identifier: `DollarLint.DollarLint`
- Package version: `0.1.3`
- Selected installer URL: `https://github.com/dollarlint/dollarlint/releases/download/v0.1.3/dollarlint_0.1.3_windows_arm64.zip`

## Script fixes made during testing

The `scripts/test-winget-manifest.ps1` script had several issues that made diagnosis harder or caused unrelated failures before the actual install test:

1. It downloaded the entire `dollarlint/winget-pkgs` branch ZIP and recursively searched it for the manifest. This was very slow for a `winget-pkgs` fork and appeared to hang. The script now uses the GitHub Contents API to download only the three manifest YAML files.
2. It passed `--id DollarLint.DollarLint` together with `winget install --manifest`. WinGet rejects that combination with "Both local manifest and search query arguments are provided." The install command now relies on `--manifest` only.
3. It enabled only `LocalManifestFiles`. The script now also enables `LocalArchiveMalwareScanOverride` so `--ignore-local-archive-malware-scan` can be used for local archive manifests.
4. Its error handling surfaced only decimal exit codes and PowerShell exception locations. The script now reports hex WinGet codes, known error descriptions, the failed command, the latest WinGet log path, and the last WinGet log line. It also exits cleanly with status `1` instead of printing a duplicate PowerShell exception block.

## Successful checks

The following parts of the test completed successfully:

- WinGet was available and reported version `v1.28.240`.
- The manifest files were downloaded from the `dollarlint-0.1.3` branch:
  - `DollarLint.DollarLint.installer.yaml`
  - `DollarLint.DollarLint.locale.en-US.yaml`
  - `DollarLint.DollarLint.yaml`
- `winget validate --manifest <manifest path>` completed with:

  ```text
  Manifest validation succeeded.
  ```

- The required admin settings were enabled:

  ```text
  Enabled admin setting 'LocalManifestFiles'.
  Enabled admin setting 'LocalArchiveMalwareScanOverride'.
  ```

- WinGet selected the expected ARM64 installer.
- The ARM64 ZIP downloaded successfully.
- The installer hash verification succeeded.

## Failure details

The install command fails after hash verification:

```text
Command failed: WinGet internal error (0x8A150001)
Command: winget install --manifest <temp manifest path> --accept-source-agreements --accept-package-agreements --ignore-local-archive-malware-scan --verbose-logs --disable-interactivity
```

The final line in the referenced WinGet log is consistently:

```text
[CORE] Started applying motw using IAttachmentExecute to <temp WinGet download path>
```

No later extraction, portable registration, or command-alias setup appears in the log. This means the failure happens before WinGet attempts to extract the ZIP or run/check `dollarlint.exe`.

## Archive sanity check

The selected ARM64 ZIP was manually downloaded and extracted successfully. The archive contains `dollarlint.exe` at the ZIP root:

```text
dollarlint.exe
LICENSE
README.md
examples\
schemas\
```

That matches the manifest's nested portable entry:

```yaml
NestedInstallerType: portable
NestedInstallerFiles:
  - RelativeFilePath: dollarlint.exe
    PortableCommandAlias: dollarlint
```

This reduces the likelihood that the failure is caused by a bad archive shape or an incorrect `RelativeFilePath`.

## Assessment

This should not be considered a full WinGet distribution success yet. It is a successful manifest validation and successful download/hash verification, but the local-manifest install test fails before installation completes.

The current evidence points away from a DollarLint-specific issue:

- The manifest validates.
- WinGet selects the correct architecture.
- The ZIP downloads.
- The SHA256 hash verifies.
- The ZIP can be manually extracted.
- `dollarlint.exe` exists at the expected root-relative path.
- The failure occurs inside Desktop App Installer's MOTW/`IAttachmentExecute` handling.

The most likely cause is a WinGet/Desktop App Installer/SmartScreen issue on this environment or this WinGet version. It may still be worth testing on another Windows machine or another WinGet/Desktop App Installer version to confirm whether the issue reproduces outside this machine.

## Recommended next steps

1. Re-run `scripts/test-winget-manifest.ps1 -UninstallAfter` on another Windows machine, ideally x64 and/or a different Desktop App Installer version.
2. If the failure reproduces, file an issue with `microsoft/winget-cli` including:
   - WinGet version: `v1.28.240`
   - Desktop App Installer version: `1.28.240.0`
   - OS build: `10.0.26200.8246`
   - Architecture: ARM64
   - Error code: `0x8A150001`
   - The full `winget install --manifest ... --verbose-logs` command
   - The final log line showing `Started applying motw using IAttachmentExecute`
   - The installer URL and the fact that hash verification succeeds
3. Treat the manifest as syntactically valid, but do not treat the local install test as complete until the install finishes and `dollarlint --version` succeeds through the WinGet-installed command alias.
