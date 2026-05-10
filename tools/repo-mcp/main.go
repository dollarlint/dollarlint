package main

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"
)

const serverName = "DollarLint Tools"

type repoServer struct {
	root                 string
	mcp                  *server.MCPServer
	realWorldRuns        *realWorldRunRegistry
	realWorldPrepareRuns *realWorldPrepareRegistry
}

func main() {
	root, err := findRepoRoot()
	if err != nil {
		log.Fatal(err)
	}
	rs := &repoServer{
		root:                 root,
		realWorldRuns:        newRealWorldRunRegistry(),
		realWorldPrepareRuns: newRealWorldPrepareRegistry(),
	}
	rs.mcp = server.NewMCPServer(
		serverName,
		"0.1.0",
		server.WithInstructions("Repo-only maintenance tools for the dollarlint checkout. These tools run curated verification, release, example, real-world corpus, and Azure diagnostics workflows for Codex sessions working in this repository."),
		server.WithToolCapabilities(false),
		server.WithInputSchemaValidation(),
		server.WithOutputSchemaValidation(),
		server.WithRecovery(),
	)
	rs.addTools()
	stdio := server.NewStdioServer(rs.mcp)
	stdio.SetErrorLogger(log.New(io.Discard, "", 0))
	if err := stdio.Listen(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
