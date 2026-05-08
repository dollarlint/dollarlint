---
title: Introduction
description: What dollarlint is and when to reach for it.
---

`dollarlint` validates source JSON, YAML, and TOML files against the JSON
Schema each file declares. It is designed to be dropped into existing
repositories: files without a schema declaration are skipped, but still
counted in the run summary so CI output makes discovery behavior clear.

## Why dollarlint

- **One tool for three formats.** A single CLI that understands the
  conventions used by editors and language servers for JSON, YAML, and TOML.
- **Schema-driven, not rule-driven.** There are no opinions about your files
  beyond what their declared schemas describe.
- **Pluggable in CI.** Text output for humans, `--json` for tooling, and
  `--sarif` for GitHub code scanning and similar systems.
- **Embeddable.** A small Go SDK exposes the same engine that powers the
  CLI.

## How it works

For each discovered file, `dollarlint`:

1. Looks for an in-file schema declaration using the conventions documented
   in [Schema declarations](/guides/schema-declarations/).
2. If none is present, falls back to any matching `[[schema.associations]]`
   in `.dollarlint.toml`.
3. If still unmatched and SchemaStore inference is enabled, tries to match
   the file against the SchemaStore catalog.
4. Compiles the resolved schema and validates the file, producing structured
   issues with optional source locations.

Files that remain unmatched are skipped, not errors.

## Next steps

- [Install](/guides/installation/) the CLI.
- Run a [quick start](/guides/quick-start/) against your repository.
- Learn the supported [schema declarations](/guides/schema-declarations/).
