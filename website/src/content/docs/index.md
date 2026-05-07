---
title: dollarlint
description: Check your JSON, YAML, and TOML files against their `$schema`s, both locally and in CI.
template: splash
hero:
  title: Schema validation for source files that carry their own contract.
  tagline: Check your JSON, YAML, and TOML files against their `$schema`s, both locally and in CI.
  actions:
    - text: Get started
      link: /dollarlint/guides/getting-started/
      icon: right-arrow
    - text: View on GitHub
      link: https://github.com/agorischek/dollarlint
      icon: external
      variant: minimal
---

<div class="signal-grid">
  <div class="signal-card">
    <strong>Finds the files that matter</strong>
    JSON, YAML, and TOML discovery with configurable include and exclude globs, plus smart defaults for non-source directories.
  </div>
  <div class="signal-card">
    <strong>Reads real editor conventions</strong>
    Supports root <code>$schema</code>, YAML language server comments, and Taplo / Even Better TOML schema directives.
  </div>
  <div class="signal-card">
    <strong>Built for CI signal</strong>
    Text, JSON, and SARIF output make local runs readable and code-scanning uploads straightforward.
  </div>
</div>

<p class="command-strip">go install github.com/agorischek/dollarlint/cmd/dollarlint@latest</p>

## Why dollarlint?

Configuration files often ship with a schema, but teams only discover drift when an editor complains or a downstream tool fails. `dollarlint` makes those schema declarations enforceable in normal development workflows.

It validates each source file against the schema it declares, skips files without a schema, reports known issue metadata, and exits non-zero when non-ignored issues remain.

## What it validates

- JSON files with a root `$schema`.
- YAML files with `# yaml-language-server: $schema=...` or a root `$schema`.
- TOML files with `#:schema ...` directives or a root `"$schema" = "..."`.
- Files matched by config-level schema associations.

## Designed for maintainers

<p class="money-note">
The palette nods to dollar bills without dressing the docs up like a novelty check: paper warmth, treasury green, dark ink, and a small gold reserve.
</p>
