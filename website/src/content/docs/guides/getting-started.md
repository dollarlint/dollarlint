---
title: Getting started
description: Install dollarlint and run your first schema validation pass.
---

## Install

Install the CLI with Go:

```sh
go install github.com/agorischek/dollarlint/cmd/dollarlint@latest
```

Then run it against a repository or config directory:

```sh
dollarlint .
```

To bootstrap a project config, run:

```sh
dollarlint init
```

That starts a short terminal interview and creates `.dollarlint.toml` in the current directory. It refuses to overwrite an existing config unless you confirm overwrite or pass `--force`.

## Try the examples

This repository includes example JSON, YAML, and TOML files:

```sh
dollarlint validate ./examples
```

There is also a SchemaStore suite with common real-world config files that declare remote schemas from `https://www.schemastore.org`:

```sh
dollarlint validate ./examples/schemastore --locations
```

Example text output is grouped by file:

```text
dollarlint found 8 issues in 3 files after 145ms

invalid.json
  /         required              missing property "enabled"
  /name     type                  got number, want string
  /count    minimum               minimum: got 0, want 1
  /extra    additionalProperties  additional property "extra" not allowed

Summary: 8 discovered, 7 validated, 1 skipped, 8 issues in 145ms
```

## Exit codes

- `0` means no non-ignored issues were found.
- `1` means validation, schema loading, or parsing issues were found.
- `2` means the CLI or configuration could not be processed.

## Common commands

```sh
dollarlint init
dollarlint init ./packages/api --schema-store
dollarlint init --output ./packages/api/.dollarlint.toml
dollarlint init --defaults --schema-store
dollarlint validate ./config --locations
dollarlint validate ./config --verbose
dollarlint validate ./config --json
dollarlint validate ./config --sarif > dollarlint.sarif
dollarlint validate . --include '**/*.yaml' --schema 'settings/*.toml=./schemas/settings.schema.json'
dollarlint validate . --schema-store
dollarlint validate . --schema-store --schema-store-failure error --fetch-retries 4
```
