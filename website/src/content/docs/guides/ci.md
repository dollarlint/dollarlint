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
      - run: dollarlint validate .
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
      - run: dollarlint validate . --sarif > dollarlint.sarif
      - uses: github/codeql-action/upload-sarif@v4
        if: always()
        with:
          sarif_file: dollarlint.sarif
```

## CI tips

- Keep `schema.fetchRemote = true` if your source files depend on remote schema URLs. Tune resilience under `[schema.fetch]` if your CI network is flaky.
- Use `schema.allowedDomains` in locked-down environments so only approved hosts are contacted.
- Pin remote schemas through local mirrors or `[[schema.associations]]` when reproducibility matters more than freshness.
- Prefer `[[ignore]]` rules for known migration debt over excluding whole directories — new issues in the same files remain visible.
- Run with `--show-skipped` periodically to confirm discovery still matches your expectations.
- Cache the Go install across runs with `actions/setup-go`'s built-in cache to keep the install step under a second.
