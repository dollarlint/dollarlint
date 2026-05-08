package dollarlint

import (
	"context"

	"github.com/agorischek/dollarlint/internal/engine"
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

func FormatJSON(result Result) ([]byte, error) {
	return engine.FormatJSON(result)
}

func FormatText(result Result, output OutputConfig) string {
	return engine.FormatText(result, output)
}

func FormatSARIF(result Result) ([]byte, error) {
	return engine.FormatSARIF(result)
}
