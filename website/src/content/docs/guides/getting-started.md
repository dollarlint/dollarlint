---
title: Getting started
description: Install dollarlint and run your first schema validation pass.
---

## Install

Install the CLI with Go:

```sh
go install github.com/agorischek/dollarlint/cmd/dollarlint@latest
```

Then run it against a repository or directory:

```sh
dollarlint .
```

`validate` is the canonical command. `dollarlint [path]` is a backwards-compatible shortcut for `dollarlint validate [path]`, so either form works in scripts and CI.

## Bootstrap a project config

Most projects work with sensible defaults. When you want a checked-in config:

```sh
dollarlint init
```

This starts a short terminal interview and writes `.dollarlint.toml` in the current directory. It refuses to overwrite an existing config unless you confirm or pass `--force`. Pass `--defaults` to skip the prompts and accept the defaults plus any flags you supply.

## Try the examples

This repository ships with example JSON, YAML, and TOML files:

```sh
dollarlint validate ./examples
```

A larger SchemaStore suite exercises common real-world config files against schemas hosted at `https://www.schemastore.org`:

```sh
dollarlint validate ./examples/schemastore --locations
```

Default text output is grouped by file:

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

| Code | Meaning |
| ---- | ------- |
| `0`  | No non-ignored issues were found. |
| `1`  | Validation, schema loading, or parsing issues were found. |
| `2`  | The CLI or configuration could not be processed. |

## Common commands

```sh
# Bootstrap configs
dollarlint init
dollarlint init ./packages/api --schema-store
dollarlint init --output ./packages/api/.dollarlint.toml
dollarlint init --defaults --schema-store

# Run validation
dollarlint validate ./config --locations
dollarlint validate ./config --verbose
dollarlint validate ./config --json
dollarlint validate ./config --sarif > dollarlint.sarif

# Narrow discovery and pin schemas inline
dollarlint validate . \
  --include '**/*.yaml' \
  --schema 'settings/*.toml=./schemas/settings.schema.json'

# Use SchemaStore matching, with stricter failure handling and more retries
dollarlint validate . --schema-store
dollarlint validate . --schema-store --schema-store-failure error --fetch-retries 4
```
