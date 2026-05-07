---
title: Go SDK
description: Use dollarlint from Go code without shelling out to the CLI.
---

The CLI is built on a Go SDK exposed by the root package.

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

## Formatting results

Use the same formatters as the CLI:

```go
text := dollarlint.FormatText(result, cfg.Output)
jsonBytes, err := dollarlint.FormatJSON(result)
sarifBytes, err := dollarlint.FormatSARIF(result)
```

## Result shape

The `Result` includes:

- `Summary` with discovered, validated, skipped, failed, ignored, issue, and duration counts.
- `Files` with per-file status and schema source.
- `Issues` with schema keyword, property, instance location, optional source line and column, and ignore metadata.
