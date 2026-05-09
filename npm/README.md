# dollarlint

Validate JSON, JSONC, JSON5, JSON Lines, YAML, and TOML files against JSON Schemas.

## Install

```sh
npm install -g dollarlint
```

Then run:

```sh
dollarlint init
dollarlint validate .
```

The npm package installs the matching `dollarlint` release binary for your platform.

## Common Commands

```sh
dollarlint validate .
dollarlint validate ./config --locations
dollarlint validate ./config --verbose
dollarlint validate ./config --format json
dollarlint validate ./config --format sarif --output dollarlint.sarif
dollarlint validate . --schema-store
```

Use `dollarlint validate <path>` for validation runs. Bare paths are not accepted.

## Other Install Options

Homebrew:

```sh
brew install --cask dollarlint/tap/dollarlint
```

Go:

```sh
go install github.com/dollarlint/dollarlint/cmd/dollarlint@latest
```

Standalone release archives are available from [GitHub Releases](https://github.com/dollarlint/dollarlint/releases/latest).

## Documentation

Full documentation lives in the [GitHub repository](https://github.com/dollarlint/dollarlint).
