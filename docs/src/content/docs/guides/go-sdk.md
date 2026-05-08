---
title: Go SDK
description: Embed the dollarlint engine in your own Go programs.
---

The root package is a small public SDK facade around the same engine that
powers the CLI.

## Minimal example

```go
package main

import (
	"context"
	"log"

	"github.com/agorischek/dollarlint"
)

func main() {
	cfg := dollarlint.DefaultConfig()
	result, err := dollarlint.Lint(context.Background(), dollarlint.Options{
		Root:   ".",
		Config: cfg,
	})
	if err != nil {
		log.Fatal(err)
	}
	if result.HasIssues() {
		log.Fatalf("found %d issues", result.Summary.Issues)
	}
}
```

## Package layout

- `github.com/agorischek/dollarlint` — public SDK facade. Stable surface
  intended for embedding.
- `internal/engine` — validation engine.
- `internal/cli` — CLI wiring.

Future integrations such as `serve`, an LSP, and an MCP server are intended
to share the same engine without expanding the public Go API accidentally.
