---
title: Schema declarations
description: Conventions dollarlint recognizes for declaring a schema in a file.
---

`dollarlint` discovers the schema for each file from in-file conventions
that match what editors and language servers already understand.

## JSON

Use the standard root `$schema` keyword:

```json
{
  "$schema": "./schema.json",
  "name": "example"
}
```

## YAML

Either the YAML language server directive or a root `$schema` key:

```yaml
# yaml-language-server: $schema=./schema.json
name: example
```

```yaml
$schema: ./schema.json
name: example
```

## TOML

Either the Taplo / Even Better TOML directive or a root `$schema` key:

```toml
#:schema ./schema.json

name = "example"
```

```toml
"$schema" = "./schema.json"

name = "example"
```

## Config-level associations

Files that do not declare a schema themselves can still be validated using
`[[schema.associations]]` entries in `.dollarlint.toml`:

```toml
[[schema.associations]]
file = "settings/*.toml"
schema = "./schemas/settings.schema.json"
```

## Resolution order

For each file, `dollarlint` resolves a schema in this order:

1. Explicit in-file `$schema` (or YAML / TOML directive).
2. `[[schema.associations]]` entries in the config.
3. SchemaStore catalog matches, if SchemaStore inference is enabled.
4. Otherwise the file is **skipped** (counted, not failed).
