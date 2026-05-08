---
title: CI integration
description: Run dollarlint in continuous integration and upload SARIF to GitHub code scanning.
---

## Basic GitHub Actions job

```yaml
name: dollarlint

on:
  pull_request:
  push:
    branches: [main]

jobs:
  validate-schemas:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version: stable
      - run: go install github.com/agorischek/dollarlint/cmd/dollarlint@latest
      - run: dollarlint .
```

## Upload SARIF

```yaml
name: dollarlint-code-scanning

on:
  pull_request:
  push:
    branches: [main]

jobs:
  validate-schemas:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      security-events: write
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version: stable
      - run: go install github.com/agorischek/dollarlint/cmd/dollarlint@latest
      - run: dollarlint . --sarif > dollarlint.sarif
      - uses: github/codeql-action/upload-sarif@v4
        if: always()
        with:
          sarif_file: dollarlint.sarif
```

## CI tips

- Keep `schema.fetchRemote: true` if your source files depend on remote schema URLs.
- Use `schema.allowedDomains` in locked-down CI environments to fetch only from approved schema hosts.
- Pin remote schemas through local mirrors or associations when reproducibility matters more than freshness.
- Use ignore rules for known migration debt instead of excluding whole directories.
- Use `--show-skipped` periodically to confirm discovery still matches your expectations.
