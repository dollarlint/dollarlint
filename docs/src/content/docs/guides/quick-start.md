---
title: Quick start
description: Initialize a config and run your first validation.
---

## 1. Create a config

```sh
dollarlint init
```

`dollarlint init` interviews you and writes a starter `.dollarlint.toml` in
the current directory. It is safe by default and will not overwrite an
existing config unless you confirm or pass `--force`.

Useful variants:

```sh
dollarlint init ./packages/api --schema-store
dollarlint init --output ./packages/api/.dollarlint.toml
dollarlint init --defaults --schema-store
```

## 2. Validate

```sh
dollarlint validate .
```

`dollarlint [path]` is a backwards-compatible shortcut for
`dollarlint validate [path]`.

## 3. Add useful flags

```sh
dollarlint validate ./config --locations
dollarlint validate ./config --verbose
dollarlint validate ./config --json
dollarlint validate ./config --sarif > dollarlint.sarif
dollarlint validate . --include '**/*.yaml' \
  --schema 'settings/*.toml=./schemas/settings.schema.json'
dollarlint validate . --schema-store
dollarlint validate . --schema-store --schema-store-failure error
```

## 4. Wire it into CI

A typical GitHub Actions step:

```yaml
- name: dollarlint
  run: |
    go install github.com/agorischek/dollarlint/cmd/dollarlint@latest
    dollarlint validate . --sarif > dollarlint.sarif
- uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: dollarlint.sarif
```

See [Exit codes](/reference/exit-codes/) for how to interpret CI failures.
