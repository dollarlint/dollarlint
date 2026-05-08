---
title: Go SDK
description: Use dollarlint from Go code without shelling out to the CLI.
---

The CLI is a thin wrapper around a Go SDK exposed by the root package. The same engine that powers `dollarlint validate` is available for embedding in your own tools, tests, or CI plugins.

## Run a validation

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

`Lint` is context-aware. Cancel the context to short-circuit long-running validations, including in-flight remote schema fetches.

## Format results

Use the same formatters the CLI uses:

```go
text := dollarlint.FormatText(result, cfg.Output)
jsonBytes, err := dollarlint.FormatJSON(result)
sarifBytes, err := dollarlint.FormatSARIF(result)
```

`FormatText` honors `cfg.Output.Verbose`, `cfg.Output.Quiet`, `cfg.Output.Locations`, and `cfg.Output.ShowSkipped` — the same fields the corresponding CLI flags set.

## Result shape

A `Result` contains:

- **`Summary`** — discovered, validated, skipped, failed, ignored, and issue counts, plus a wall-clock duration.
- **`Files`** — per-file status and schema source (`in-file`, `association`, `schema-store`, or `skipped`).
- **`Issues`** — schema keyword, JSON pointer to the offending property, instance location, optional source line and column, and ignore metadata.
- **`Warnings`** — non-fatal events such as a temporarily unavailable SchemaStore catalog.

Use `result.HasIssues()` for the same exit-code-friendly signal the CLI uses: it returns `true` only when there are non-ignored validation issues.
