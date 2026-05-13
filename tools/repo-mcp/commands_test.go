package main

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRepoCommandShellFallsBackToSh(t *testing.T) {
	shell, args := repoCommandShellWith("", func(name string) bool {
		return name == "/bin/sh"
	})
	if shell != "/bin/sh" {
		t.Fatalf("shell = %q, want /bin/sh", shell)
	}
	if len(args) != 1 || args[0] != "-c" {
		t.Fatalf("args = %v, want [-c]", args)
	}
}

func TestRepoCommandShellPreservesLoginShellForZshOnly(t *testing.T) {
	for _, shell := range []string{"/bin/zsh"} {
		t.Run(filepath.Base(shell), func(t *testing.T) {
			gotShell, args := repoCommandShellWith(shell, func(name string) bool {
				return name == shell
			})
			if gotShell != shell {
				t.Fatalf("shell = %q, want %q", gotShell, shell)
			}
			if len(args) != 1 || args[0] != "-lc" {
				t.Fatalf("args = %v, want [-lc]", args)
			}
		})
	}
}

func TestRepoCommandShellUsesNonLoginBash(t *testing.T) {
	shell, args := repoCommandShellWith("/bin/bash", func(name string) bool {
		return name == "/bin/bash"
	})
	if shell != "/bin/bash" {
		t.Fatalf("shell = %q, want /bin/bash", shell)
	}
	if len(args) != 1 || args[0] != "-c" {
		t.Fatalf("args = %v, want [-c]", args)
	}
}

func TestRunUsesConfiguredPOSIXShell(t *testing.T) {
	t.Setenv("DOLLARLINT_MCP_SHELL", "/bin/sh")
	server := &repoServer{root: t.TempDir()}
	result := server.run(context.Background(), namedCommand{Name: "printf", Cmd: "printf ok"})
	if !result.Succeeded {
		t.Fatalf("run failed: %+v", result)
	}
	if result.Output != "ok" {
		t.Fatalf("output = %q, want ok", result.Output)
	}
}
