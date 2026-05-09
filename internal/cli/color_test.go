package cli

import (
	"strings"
	"testing"

	"github.com/dollarlint/dollarlint"
	"github.com/orochaa/go-clack/third_party/picocolors"
)

func TestConfigureNoColorDisablesCLIStyling(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	configureNoColor()

	if got := picocolors.Green("prompt"); got != "prompt" {
		t.Fatalf("clack color output = %q", got)
	}

	text := dollarlint.FormatText(dollarlint.Result{}, dollarlint.OutputConfig{})
	if strings.Contains(text, "\x1b[") {
		t.Fatalf("text output contains ANSI escape sequences: %q", text)
	}
}
