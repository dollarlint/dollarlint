# SchemaStore examples

These examples declare schemas from `https://www.schemastore.org` and intentionally contain schema violations so dollarlint's error reporting can be exercised against remote schemas.

```sh
dollarlint validate ./examples/schemastore --locations
```

The files cover common JSON, YAML, and TOML configuration formats that developers often already validate through editor integrations backed by SchemaStore.
