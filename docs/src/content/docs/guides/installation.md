---
title: Installation
description: Install the dollarlint CLI.
---

## With `go install`

```sh
go install github.com/agorischek/dollarlint/cmd/dollarlint@latest
```

This installs the `dollarlint` binary into `$GOBIN` (or `$GOPATH/bin`). Make
sure that directory is on your `PATH`.

## From source

```sh
git clone https://github.com/agorischek/dollarlint.git
cd dollarlint
go build ./cmd/dollarlint
```

## Verify the install

```sh
dollarlint --help
dollarlint validate --help
```
