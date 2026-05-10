package main

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestToolNameFilterAllowsEverythingWhenUnset(t *testing.T) {
	filter := parseToolNameFilter("")
	for _, name := range []string{"real_world_start_testing", "repo_status"} {
		if !filter.Allows(name) {
			t.Fatalf("empty filter should allow %s", name)
		}
	}
}

func TestToolNameFilterSupportsGlobPatterns(t *testing.T) {
	filter := parseToolNameFilter("real_world_*,repo_status")
	allowed := []string{"real_world_start_testing", "real_world_record_result", "repo_status"}
	for _, name := range allowed {
		if !filter.Allows(name) {
			t.Fatalf("filter should allow %s", name)
		}
	}
	if filter.Allows("verify") {
		t.Fatalf("filter should not allow verify")
	}
}

func TestToolNameFilterSplitsCommaAndWhitespace(t *testing.T) {
	filter := parseToolNameFilter("real_world_start_testing repo_status,\nverify")
	allowed := []string{"real_world_start_testing", "repo_status", "verify"}
	for _, name := range allowed {
		if !filter.Allows(name) {
			t.Fatalf("filter should allow %s", name)
		}
	}
	if filter.Allows("release_status") {
		t.Fatalf("filter should not allow release_status")
	}
}

func TestRepoServerToolFilterRegistersOnlyMatchingTools(t *testing.T) {
	rs := &repoServer{toolFilter: parseToolNameFilter("real_world_*")}
	rs.mcp = server.NewMCPServer(serverName, "test")
	rs.addTools()
	tools := rs.mcp.ListTools()
	if len(tools) == 0 {
		t.Fatalf("expected real-world tools to be registered")
	}
	for name := range tools {
		if !strings.HasPrefix(name, "real_world_") {
			t.Fatalf("registered non-real-world tool %s", name)
		}
	}
	if _, ok := tools["real_world_start_testing"]; !ok {
		t.Fatalf("missing real_world_start_testing")
	}
	if _, ok := tools["repo_status"]; ok {
		t.Fatalf("repo_status should not be registered")
	}
}
