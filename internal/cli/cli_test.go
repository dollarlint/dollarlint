package cli

import (
	"bufio"
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
	if code := Execute([]string{"validate", dir, "--json"}, &stdout, &stderr); code != 1 {
		t.Fatalf("validate command exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"issues": 1`) {
		t.Fatalf("validate command json output missing issue count: %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"lint", dir, "--json"}, &stdout, &stderr); code != 1 {
		t.Fatalf("lint alias exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"issues": 1`) {
		t.Fatalf("lint alias json output missing issue count: %s", stdout.String())
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

func TestExecuteSchemaStoreFlags(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "catalog.json"), `{"schemas":[{"name":"Custom","fileMatch":["custom.json"],"url":"./custom.schema.json"}]}`)
	writeFile(t, filepath.Join(dir, "custom.schema.json"), `{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}`)
	writeFile(t, filepath.Join(dir, "custom.json"), `{"ok":true}`)
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"validate", dir, "--schema-store-url", filepath.Join(dir, "catalog.json"), "--schema-store-strict", "--fetch-retries", "1", "--fetch-retry-min-wait", "1ms", "--fetch-retry-max-wait", "1ms", "--show-skipped"}, &stdout, &stderr); code != 0 {
		t.Fatalf("schema-store run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 validated") || !strings.Contains(stdout.String(), "2 skipped") {
		t.Fatalf("schema-store output = %s", stdout.String())
	}
}

func TestInitCommandCreatesStarterConfig(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"init", dir, "--schema-store"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "No interactive terminal detected") {
		t.Fatalf("noninteractive init should explain defaults: %s", stdout.String())
	}
	configPath := filepath.Join(dir, ".dollarlint.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "[schema.schemaStore]") || !strings.Contains(text, "enabled = true") || !strings.Contains(text, `retryMinWait = "250ms"`) {
		t.Fatalf("generated toml = %s", text)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"init", dir}, &stdout, &stderr); code != 2 {
		t.Fatalf("expected existing config failure, exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Fatalf("existing config stderr = %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"init", dir, "--force", "--schema-store-strict"}, &stdout, &stderr); code != 0 {
		t.Fatalf("force init exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read forced config: %v", err)
	}
	if !strings.Contains(string(data), "strict = true") {
		t.Fatalf("forced config = %s", string(data))
	}
	stdout.Reset()
	stderr.Reset()
	defaultsDir := filepath.Join(dir, "defaults")
	if code := Execute([]string{"init", defaultsDir, "--defaults"}, &stdout, &stderr); code != 0 {
		t.Fatalf("defaults init exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "No interactive terminal detected") {
		t.Fatalf("--defaults should silence noninteractive explanation: %s", stdout.String())
	}
}

func TestInitCommandRequiresTOML(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	tomlPath := filepath.Join(dir, "nested", ".dollarlint.toml")
	if code := Execute([]string{"init", "--output", tomlPath, "--schema-store-strict"}, &stdout, &stderr); code != 0 {
		t.Fatalf("toml init exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatalf("read toml config: %v", err)
	}
	if !strings.Contains(string(data), "[schema.schemaStore]") || !strings.Contains(string(data), "strict = true") {
		t.Fatalf("toml config = %s", string(data))
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"init", dir, "--output", "dollarlint.toml"}, &stdout, &stderr); code != 2 {
		t.Fatalf("expected unsupported inferred format failure, exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), ".dollarlint.toml") {
		t.Fatalf("wrong-name init stderr = %s", stderr.String())
	}
}

func TestInitPlainPrompts(t *testing.T) {
	input := bufio.NewReader(strings.NewReader("n\n4\n\ny\n"))
	opts := defaultInitOptions()
	var stdout bytes.Buffer
	if err := interviewInit(input, &stdout, &opts); err != nil {
		t.Fatalf("interviewInit: %v", err)
	}
	if opts.fetchRemote || opts.fetchRetries != 4 || !opts.schemaStore || !opts.schemaStoreStrict {
		t.Fatalf("opts = %+v", opts)
	}
	output := stdout.String()
	if strings.Contains(output, "┃") || strings.Contains(output, "\x1b[") {
		t.Fatalf("plain prompts should not include borders or ANSI styling: %q", output)
	}
	if !strings.Contains(output, "Allow remote http(s) schema fetching? [Y/n]:") {
		t.Fatalf("prompt output = %q", output)
	}
	if !strings.Contains(output, "Enable SchemaStore filename matching? [Y/n]:") {
		t.Fatalf("schemastore should default to yes: %q", output)
	}
}

func TestDefaultInitOptionsDrivePromptsAndConfig(t *testing.T) {
	opts := defaultInitOptions()
	var promptOutput bytes.Buffer
	if err := interviewInit(bufio.NewReader(strings.NewReader("\n\nn\n")), &promptOutput, &opts); err != nil {
		t.Fatalf("interviewInit: %v", err)
	}
	if !opts.fetchRemote || opts.fetchRetries != defaultFetchRetries || opts.schemaStore || opts.schemaStoreStrict {
		t.Fatalf("opts after prompts = %+v", opts)
	}
	promptText := promptOutput.String()
	if !strings.Contains(promptText, "Allow remote http(s) schema fetching? [Y/n]:") {
		t.Fatalf("fetch remote prompt should reflect defaultInitOptions: %q", promptText)
	}
	if !strings.Contains(promptText, "Retries for transient schema fetch failures [2]:") {
		t.Fatalf("retry prompt should reflect defaultInitOptions: %q", promptText)
	}
	if !strings.Contains(promptText, "Enable SchemaStore filename matching? [Y/n]:") {
		t.Fatalf("schemastore prompt should reflect defaultInitOptions: %q", promptText)
	}
	data, err := renderStarterConfig(defaultInitOptions())
	if err != nil {
		t.Fatalf("renderStarterConfig: %v", err)
	}
	config := string(data)
	for _, expected := range []string{
		"fetchRemote = true",
		`retries = 2`,
		"enabled = true",
		"strict = false",
	} {
		if !strings.Contains(config, expected) {
			t.Fatalf("default config missing %q:\n%s", expected, config)
		}
	}
}

func TestPromptValidationRepeats(t *testing.T) {
	input := bufio.NewReader(strings.NewReader("maybe\ny\n-1\n2\n"))
	var stdout bytes.Buffer
	confirmed, err := promptConfirm(input, &stdout, "Continue?", false)
	if err != nil {
		t.Fatalf("promptConfirm: %v", err)
	}
	retries, err := promptNonNegativeInt(input, &stdout, "Retries", 0)
	if err != nil {
		t.Fatalf("promptNonNegativeInt: %v", err)
	}
	if !confirmed || retries != 2 {
		t.Fatalf("confirmed=%v retries=%d", confirmed, retries)
	}
	if !strings.Contains(stdout.String(), "Please answer y or n.") || !strings.Contains(stdout.String(), "Please enter a non-negative integer.") {
		t.Fatalf("validation output = %q", stdout.String())
	}
}

func TestHelpAndVersionCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Available Commands:") || !strings.Contains(stdout.String(), "validate") || !strings.Contains(stdout.String(), "init") {
		t.Fatalf("help output = %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "\n  lint") {
		t.Fatalf("hidden lint alias should not appear in help output: %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version command exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "dollarlint version dev" {
		t.Fatalf("version command output = %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version flag exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "dollarlint version dev") {
		t.Fatalf("version flag output = %q", stdout.String())
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
