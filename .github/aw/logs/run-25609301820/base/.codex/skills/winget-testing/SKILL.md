---
name: winget-testing
description: DollarLint WinGet validation workflow. Use when Codex is asked to test, update, diagnose, or explain DollarLint's WinGet manifest, Microsoft winget-pkgs PR validation, Windows install behavior, or scripts/test-winget-manifest.ps1.
---

# WinGet Testing

## Core Workflow

1. Check the DollarLint release and WinGet PR context first.
   - Confirm the package version under discussion.
   - Confirm the manifest branch in `dollarlint/winget-pkgs`; release branches are normally named `dollarlint-x.y.z`.
   - If checking a Microsoft PR, inspect the PR checks, labels, comments, and the linked Azure validation run before drawing conclusions.

2. Use `scripts/test-winget-manifest.ps1` for local Windows validation.
   - The script validates the manifest, enables required local-manifest admin settings, installs from the local manifest, checks `winget list`, checks `dollarlint --version`, and can uninstall afterward.
   - Run from elevated Administrator PowerShell on Windows:

```powershell
.\scripts\test-winget-manifest.ps1 -UninstallAfter
```

   - By default, the script discovers the newest `dollarlint-x.y.z` branch in `dollarlint/winget-pkgs`.
   - For reproducible release checks, pass the branch or version explicitly:

```powershell
.\scripts\test-winget-manifest.ps1 `
  -Branch dollarlint-0.1.5 `
  -PackageVersion 0.1.5 `
  -UninstallAfter
```

3. Interpret the result carefully.
   - Full success means manifest validation, local WinGet install, command alias registration, `dollarlint --version`, and optional uninstall all completed.
   - Manifest validation success alone is useful but is not a full WinGet install success.
   - A failure after download and SHA256 verification at `IAttachmentExecute` / Mark-of-the-Web is likely a Desktop App Installer, SmartScreen, or WinGet environment issue before archive extraction. Do not describe that as a DollarLint binary or manifest failure without more evidence.

4. Preserve useful diagnostics.
   - Capture OS version, CPU architecture, WinGet version, Desktop App Installer version, package version, branch, installer URL, failure exit code, latest WinGet log path, and the last relevant log lines.
   - If the script fails, ask for the full script output and the latest WinGet log path it prints.
   - Update `WINGET-TEST.md` when a new Windows environment produces a materially new result.

5. For Microsoft `winget-pkgs` PR validation, separate PR state from service state.
   - A stale `Needs-CLA` label can coexist with a green `license/cla` check. Trust the current check result more than a stale label.
   - `Internal-Error-Dynamic-Scan` can be caused by Microsoft service-side scanning of unrelated Windows executables. Inspect validation artifacts before assuming DollarLint caused it.
   - Keep PR comments factual: state what passed, what failed, and what remains unverified.

## Useful Commands

Check the current Microsoft PR:

```bash
gh pr view <number> --repo microsoft/winget-pkgs \
  --json number,title,url,state,isDraft,labels,statusCheckRollup,comments,updatedAt
```

Check a linked Azure validation build:

```bash
curl -sS 'https://dev.azure.com/shine-oss/winget-pkgs/_apis/build/builds/<id>?api-version=7.1-preview.7' |
  jq '{id,buildNumber,status,result,queueTime,startTime,finishTime,url:._links.web.href}'
```

List Azure validation artifacts:

```bash
curl -sS 'https://dev.azure.com/shine-oss/winget-pkgs/_apis/build/builds/<id>/artifacts?api-version=7.1-preview.5' |
  jq -r '.value[]? | [.name,.resource.type,.resource.downloadUrl] | @tsv'
```
