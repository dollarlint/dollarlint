---
title: CLI reference
description: Commands and flags exposed by the dollarlint CLI.
---

## Commands

### `dollarlint init`

Interactively scaffold a `.dollarlint.toml` in the target directory. Safe
by default; will not overwrite an existing config without confirmation or
`--force`.

```sh
dollarlint init
dollarlint init ./packages/api --schema-store
dollarlint init --output ./packages/api/.dollarlint.toml
dollarlint init --defaults --schema-store
```

Selected flags:

| Flag             | Description                                                       |
| ---------------- | ----------------------------------------------------------------- |
| `--output`       | Write the config to a specific path.                              |
| `--defaults`     | Use defaults for every prompt.                                    |
| `--schema-store` | Enable SchemaStore inference in the generated config.             |
| `--force`        | Overwrite an existing config without prompting.                   |

### `dollarlint validate [path]`

Validate files under `path` (defaults to `.`). Bare paths are intentionally
not accepted; use the explicit `validate` command.

```sh
dollarlint validate .
dollarlint validate ./config --locations
dollarlint validate ./config --verbose
dollarlint validate ./config --json
dollarlint validate ./config --sarif > dollarlint.sarif
dollarlint validate . --include '**/*.yaml' \
  --schema 'settings/*.toml=./schemas/settings.schema.json'
dollarlint validate . --schema-store
dollarlint validate . --schema-store --schema-store-failure error
```

Selected flags:

| Flag                         | Description                                                                                  |
| ---------------------------- | -------------------------------------------------------------------------------------------- |
| `--include <glob>`           | Restrict discovery to files matching one or more globs. Repeatable.                          |
| `--schema <glob>=<path>`     | Add a config-style association on the command line. Repeatable.                              |
| `--schema-store`             | Enable SchemaStore inference for unmatched files.                                            |
| `--schema-store-failure`     | One of `warn`, `error`, `skip`. Controls how SchemaStore catalog failures are handled.       |
| `--locations`                | Include line/column source mapping in text and JSON output.                                  |
| `--verbose`                  | Show schema URI and keyword metadata under each issue.                                       |
| `--quiet`                    | Terse success output.                                                                        |
| `--json`                     | Emit machine-readable JSON.                                                                  |
| `--sarif`                    | Emit SARIF 2.1.0 suitable for GitHub code scanning.                                          |

See [Exit codes](/reference/exit-codes/) for how runs terminate.
