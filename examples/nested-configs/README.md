# Nested config example

This example shows a parent run that applies the nearest `.dollarlint.toml` for
each file.

```sh
dollarlint validate ./examples/nested-configs
```

The root config enables nearest mode and validates `root.json` with
`schemas/root.schema`. The `packages/api` config extends the root config but
overrides the schema association for files in that subtree.
