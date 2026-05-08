---
title: SchemaStore inference
description: Match unknown files against the SchemaStore catalog.
---

When SchemaStore inference is enabled, files without an explicit schema can
be matched by conventional filename using the public SchemaStore catalog or
a local SchemaStore-shaped catalog.

## Enable from the CLI

```sh
dollarlint validate . --schema-store
```

## Enable from config

```toml
[schema.schemaStore]
enabled = true
url = "https://www.schemastore.org/api/json/catalog.json"
failure = "warn"
strict = false
```

## Failure modes

Catalog availability problems are modeled separately from validation
issues. The `schema.schemaStore.failure` setting controls behavior:

- `"warn"` (default): record a warning, skip SchemaStore inference, still
  validate explicit and configured schemas, and exit `0` unless validation
  issues are found.
- `"error"`: fail the run with exit code `2` when the catalog cannot be
  loaded.
- `"skip"`: silently fall back, matching the historical behavior.

`schema.schemaStore.strict = true` is supported as a legacy alias for
`"error"`.

```sh
dollarlint validate . --schema-store --schema-store-failure error
```

## Allow- and block-lists

Remote schema fetching is enabled by default. Restrict it using
`schema.allowedDomains` and `schema.blockedDomains`:

```toml
[schema]
allowedDomains = ["www.schemastore.org", "raw.githubusercontent.com"]
blockedDomains = ["untrusted.example.com"]
```

Leave `allowedDomains` empty to allow any host. Entries support exact host
names like `schemas.example.com` and wildcard hosts like `*.example.com`.

## Azure Resource Manager pruning

Azure Resource Manager deployment schemas from `schema.management.azure.com`
are pruned to the resource provider schemas the template actually uses
before compilation. Disable this with `schema.azureResourcePruning = false`
if you need the full Azure provider catalog.
