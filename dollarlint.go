package dollarlint

import (
	"context"

	"github.com/dollarlint/dollarlint/internal/engine"
)

func DefaultConfig() Config {
	return engine.DefaultConfig()
}

func LoadConfig(root, explicitPath string) (Config, string, error) {
	return engine.LoadConfig(root, explicitPath)
}

func Lint(ctx context.Context, opts Options) (Result, error) {
	return engine.Lint(ctx, opts)
}

func Inspect(ctx context.Context, opts Options) (InspectResult, error) {
	return engine.Inspect(ctx, opts)
}

func FormatJSON(result Result) ([]byte, error) {
	return engine.FormatJSON(result)
}

func FormatInspectJSON(result InspectResult) ([]byte, error) {
	return engine.FormatInspectJSON(result)
}

func FormatText(result Result, output OutputConfig) string {
	return engine.FormatText(result, output)
}

func FormatInspectText(result InspectResult) string {
	return engine.FormatInspectText(result)
}

func FormatSARIF(result Result) ([]byte, error) {
	return engine.FormatSARIF(result)
}

func FormatBundle(result Result, output OutputConfig) ([]byte, error) {
	return engine.FormatBundle(result, output)
}
