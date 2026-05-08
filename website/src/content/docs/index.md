---
title: dollarlint
description: Validate JSON, YAML, and TOML files against the JSON Schema each file declares, locally and in CI.
template: splash
hero:
  title: Schema validation for files that carry their own contract.
  tagline: Validate JSON, YAML, and TOML against the JSON Schema each file declares — locally, in CI, and from Go.
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
    JSON, YAML, and TOML discovery with configurable include and exclude globs, and sensible defaults that skip <code>node_modules</code>, <code>dist</code>, and <code>.git</code>.
  </div>
  <div class="signal-card">
    <strong>Reads real editor conventions</strong>
    Honors root <code>$schema</code>, the <code>yaml-language-server</code> comment, and the Taplo / Even Better TOML <code>#:schema</code> directive.
  </div>
  <div class="signal-card">
    <strong>Built for CI signal</strong>
    Text, JSON, and SARIF output keep local runs readable and make GitHub code-scanning uploads a one-liner.
  </div>
</div>

<p class="command-strip"><span class="command-prompt">$</span> go install github.com/agorischek/dollarlint/cmd/dollarlint@latest</p>

## Why dollarlint?

Configuration files often ship with a schema, but teams only notice drift when an editor complains or a downstream tool fails. `dollarlint` turns those in-file schema declarations into something you can enforce in normal development workflows.

It validates each source file against the schema it declares, skips files without one, reports keyword-level issue metadata, and exits non-zero when any non-ignored issue remains.

## What it validates

- JSON files with a root `$schema`.
- YAML files with `# yaml-language-server: $schema=...` or a root `$schema`.
- TOML files with `#:schema ...` directives or a root `"$schema" = "..."`.
- Any file matched by a config-level schema association or a SchemaStore filename rule.

## Where to go next

- **[Getting started](/dollarlint/guides/getting-started/)** — install, run, and read the output.
- **[Configuration](/dollarlint/guides/configuration/)** — discovery, schema loading, ignore rules, and timeouts.
- **[Output formats](/dollarlint/guides/output-formats/)** — text, JSON, SARIF, and locations.
- **[CI integration](/dollarlint/guides/ci/)** — drop-in GitHub Actions jobs, including SARIF upload.
- **[Go SDK](/dollarlint/guides/go-sdk/)** — embed the validator in your own Go programs.
