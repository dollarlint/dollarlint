package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteExitCodes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schema.json"), `{"type":"object","required":["name"],"properties":{"$schema":{"type":"string"},"name":{"type":"string"}}}`)
	writeFile(t, filepath.Join(dir, "bad.json"), `{"$schema":"./schema.json"}`)
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{dir, "--json"}, &stdout, &stderr); code != 1 {
		t.Fatalf("invalid run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"issues": 1`) {
		t.Fatalf("json output missing issue count: %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{dir, "--sarif"}, &stdout, &stderr); code != 1 {
		t.Fatalf("sarif run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"version": "2.1.0"`) || !strings.Contains(stdout.String(), `"startLine"`) {
		t.Fatalf("sarif output mismatch: %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{filepath.Join(dir, "missing")}, &stdout, &stderr); code != 2 {
		t.Fatalf("fatal run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestExecuteSuccessAndHelpers(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plain.json"), `{}`)
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{dir, "--show-skipped"}, &stdout, &stderr); code != 0 {
		t.Fatalf("success exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "skipped: plain.json") {
		t.Fatalf("text output = %s", stdout.String())
	}
	if _, err := parseAssociation("nope"); err == nil {
		t.Fatalf("expected association parse error")
	}
	association, err := parseAssociation("*.json=./schema.json")
	if err != nil {
		t.Fatalf("parseAssociation: %v", err)
	}
	if association.File != "*.json" || association.Schema != "./schema.json" {
		t.Fatalf("association = %+v", association)
	}
}

func TestExecuteRemoteDomainFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"type":"object"}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "file.json"), `{"$schema":"`+server.URL+`/schema.json"}`)
	host := mustHost(t, server.URL)
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{dir, "--allow-domain", host}, &stdout, &stderr); code != 0 {
		t.Fatalf("allowed run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{dir, "--block-domain", host}, &stdout, &stderr); code != 1 {
		t.Fatalf("blocked run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "blocked by configuration") {
		t.Fatalf("blocked output = %s", stdout.String())
	}
}

func mustHost(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return parsed.Hostname()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
