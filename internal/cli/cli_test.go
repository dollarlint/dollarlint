package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteExitCodes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schema.json"), `{"type":"object","required":["name"],"properties":{"$schema":{"type":"string"},"name":{"type":"string"}}}`)
	writeFile(t, filepath.Join(dir, "bad.json"), `{"$schema":"./schema.json"}`)
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{dir, "--format", "json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("bare path exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown command") && !strings.Contains(stderr.String(), "unknown flag") {
		t.Fatalf("bare path stderr = %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", dir, "--format", "json"}, &stdout, &stderr); code != 1 {
		t.Fatalf("validate command exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"issues": 1`) {
		t.Fatalf("validate command json output missing issue count: %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"lint", dir}, &stdout, &stderr); code != 2 {
		t.Fatalf("lint shortcut exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("lint shortcut stderr = %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", dir, "--format", "sarif"}, &stdout, &stderr); code != 1 {
		t.Fatalf("sarif run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"version": "2.1.0"`) || !strings.Contains(stdout.String(), `"startLine"`) {
		t.Fatalf("sarif output mismatch: %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	outputPath := filepath.Join(dir, "dollarlint.sarif")
	if code := Execute([]string{"validate", dir, "--format", "sarif", "--output", outputPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("sarif file run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected --output to suppress stdout, got %s", stdout.String())
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read sarif output: %v", err)
	}
	if !strings.Contains(string(data), `"version": "2.1.0"`) || !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("sarif file output mismatch: %s", string(data))
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", dir, "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("removed json shortcut exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown flag") {
		t.Fatalf("removed json shortcut stderr = %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", dir, "--format", "xml"}, &stdout, &stderr); code != 2 {
		t.Fatalf("invalid format exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown output format") {
		t.Fatalf("invalid format stderr = %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	jsoncDir := filepath.Join(dir, "jsonc")
	writeFile(t, filepath.Join(jsoncDir, "schema.json"), `{"type":"object","required":["ok"],"properties":{"$schema":{"type":"string"},"ok":{"type":"boolean"}}}`)
	writeFile(t, filepath.Join(jsoncDir, "settings.jsonc"), `{"$schema":"./schema.json", "ok": true,}`)
	if code := Execute([]string{"validate", filepath.Join(jsoncDir, "settings.jsonc"), "--format", "json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("jsonc explicit exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"format": "jsonc"`) {
		t.Fatalf("jsonc explicit output = %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", filepath.Join(dir, "missing")}, &stdout, &stderr); code != 2 {
		t.Fatalf("fatal run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	writeFile(t, filepath.Join(dir, "README.md"), "# docs\n")
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", filepath.Join(dir, "README.md")}, &stdout, &stderr); code != 2 {
		t.Fatalf("unsupported explicit file exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "unsupported explicit file") {
		t.Fatalf("unsupported explicit file stderr = %s", stderr.String())
	}
}

func TestExecuteSuccessAndHelpers(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plain.json"), `{}`)
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"validate", dir, "--show-skipped"}, &stdout, &stderr); code != 0 {
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
		time.Sleep(30 * time.Millisecond)
		w.Write([]byte(`{"type":"object"}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "file.json"), `{"$schema":"`+server.URL+`/schema.json"}`)
	host := mustHost(t, server.URL)
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"validate", dir, "--allow-domain", host, "--format", "json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("allowed run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var decoded struct {
		Summary struct {
			DurationNanos int64 `json:"durationNanos"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json output: %v\n%s", err, stdout.String())
	}
	if decoded.Summary.DurationNanos < int64(25*time.Millisecond) {
		t.Fatalf("printed duration did not include schema fetch: %+v", decoded.Summary)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", dir, "--block-domain", host}, &stdout, &stderr); code != 1 {
		t.Fatalf("blocked run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "blocked by configuration") {
		t.Fatalf("blocked output = %s", stdout.String())
	}
}

func TestExecuteNoSchemaCacheFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"type":"object"}`))
	}))
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "file.json"), `{"$schema":"`+server.URL+`/schema.json"}`)
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"validate", dir, "--no-schema-cache"}, &stdout, &stderr); code != 0 {
		t.Fatalf("initial no-cache run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	server.Close()
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", dir, "--no-schema-cache"}, &stdout, &stderr); code != 1 {
		t.Fatalf("second no-cache run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "fetch schema") {
		t.Fatalf("second no-cache output = %s", stdout.String())
	}
}

func TestExecuteDiscoveryFlags(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schema.json"), `{"type":"object","required":["ok"],"properties":{"$schema":{"type":"string"},"ok":{"type":"boolean"}}}`)
	writeFile(t, filepath.Join(dir, ".gitignore"), "ignored.json\n")
	writeFile(t, filepath.Join(dir, "ignored.json"), `{"$schema":"./schema.json"}`)
	writeFile(t, filepath.Join(dir, "generated", "bad.json"), `{"$schema":"../schema.json"}`)
	writeFile(t, filepath.Join(dir, "node_modules", "bad.json"), `{"$schema":"../schema.json"}`)
	writeFile(t, filepath.Join(dir, "target.json"), `{"$schema":"./schema.json","ok":true}`)
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"validate", dir, "--format", "json", "--exclude", "generated/**"}, &stdout, &stderr); code != 0 {
		t.Fatalf("default discovery exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"issues": 0`) {
		t.Fatalf("default discovery output = %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", dir, "--format", "json", "--no-gitignore", "--exclude", "generated/**"}, &stdout, &stderr); code != 1 {
		t.Fatalf("no-gitignore exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"issues": 1`) {
		t.Fatalf("no-gitignore output = %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", dir, "--format", "json", "--no-default-excludes", "--exclude", "generated/**"}, &stdout, &stderr); code != 1 {
		t.Fatalf("no-default-excludes exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"issues": 1`) {
		t.Fatalf("no-default-excludes output = %s", stdout.String())
	}
	writeFile(t, filepath.Join(dir, "target.json"), `{"$schema":"./schema.json"}`)
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", filepath.Join(dir, "target.json"), "--format", "json", "--exclude", "target.json"}, &stdout, &stderr); code != 1 {
		t.Fatalf("explicit file exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", filepath.Join(dir, "target.json"), "--format", "json", "--exclude", "target.json", "--force-exclude"}, &stdout, &stderr); code != 0 {
		t.Fatalf("force-exclude exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"discovered": 0`) {
		t.Fatalf("force-exclude output = %s", stdout.String())
	}
}

func TestExecuteSchemaStoreFlags(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "catalog.json"), `{"schemas":[{"name":"Custom","fileMatch":["custom.json"],"url":"./custom.schema.json"}]}`)
	writeFile(t, filepath.Join(dir, "custom.schema.json"), `{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}`)
	writeFile(t, filepath.Join(dir, "custom.json"), `{"ok":true}`)
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"validate", dir, "--schema-store-url", filepath.Join(dir, "catalog.json"), "--schema-store-failure", "warn", "--fetch-retries", "1", "--fetch-retry-min-wait", "1ms", "--fetch-retry-max-wait", "1ms", "--show-skipped"}, &stdout, &stderr); code != 0 {
		t.Fatalf("schema-store run exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 validated") || !strings.Contains(stdout.String(), "2 skipped") {
		t.Fatalf("schema-store output = %s", stdout.String())
	}
}

func TestExecuteRejectsNegativeNumericOptions(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"validate", dir, "--max-depth", "-1"}, &stdout, &stderr); code != 2 {
		t.Fatalf("negative max-depth exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "schemas.maxDepth") {
		t.Fatalf("negative max-depth stderr = %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"validate", dir, "--fetch-retries", "-1"}, &stdout, &stderr); code != 2 {
		t.Fatalf("negative fetch-retries exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "schemas.fetch.retries") {
		t.Fatalf("negative fetch-retries stderr = %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"init", filepath.Join(dir, "init"), "--defaults", "--fetch-retries", "-1"}, &stdout, &stderr); code != 2 {
		t.Fatalf("negative init fetch-retries exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "fetch-retries") {
		t.Fatalf("negative init fetch-retries stderr = %s", stderr.String())
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
	if !strings.Contains(stdout.String(), "Run dollarlint validate . to check your files.") || strings.Contains(stdout.String(), "Created .dollarlint.toml with") {
		t.Fatalf("init success output = %s", stdout.String())
	}
	configPath := filepath.Join(dir, ".dollarlint.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "[schemas.catalogs]") || !strings.Contains(text, "[[schemas.catalogs.sources]]") || !strings.Contains(text, "enabled = true") || !strings.Contains(text, `failure = "warn"`) || !strings.Contains(text, `retryMinWait = "250ms"`) {
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
	if code := Execute([]string{"init", dir, "--force", "--schema-store-failure", "error"}, &stdout, &stderr); code != 0 {
		t.Fatalf("force init exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read forced config: %v", err)
	}
	if !strings.Contains(string(data), `failure = "error"`) || strings.Contains(string(data), "strict =") {
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
	if code := Execute([]string{"init", "--output", tomlPath, "--schema-store-failure", "error"}, &stdout, &stderr); code != 0 {
		t.Fatalf("toml init exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatalf("read toml config: %v", err)
	}
	if !strings.Contains(string(data), "[schemas.catalogs]") || !strings.Contains(string(data), `failure = "error"`) || strings.Contains(string(data), "strict =") {
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
	opts := defaultInitOptions()
	prompter := fakeInitPrompter{
		confirms: []bool{false, true},
		ints:     []int{4},
		failures: []string{"error"},
	}
	if err := interviewInitWithPrompter(&prompter, &opts); err != nil {
		t.Fatalf("interviewInit: %v", err)
	}
	if opts.fetchRemote || opts.fetchRetries != 4 || !opts.schemaStore || opts.catalogFailure != "error" {
		t.Fatalf("opts = %+v", opts)
	}
	if strings.Join(prompter.questions, "|") != "Allow remote http(s) schema fetching?|Retries for transient schema fetch failures|Enable catalog filename matching?|catalogFailure" {
		t.Fatalf("questions = %+v", prompter.questions)
	}
}

func TestDefaultInitOptionsDrivePromptsAndConfig(t *testing.T) {
	opts := defaultInitOptions()
	prompter := fakeInitPrompter{
		confirms: []bool{true, false},
		ints:     []int{defaultFetchRetries},
	}
	if err := interviewInitWithPrompter(&prompter, &opts); err != nil {
		t.Fatalf("interviewInit: %v", err)
	}
	if !opts.fetchRemote || opts.fetchRetries != defaultFetchRetries || opts.schemaStore {
		t.Fatalf("opts after prompts = %+v", opts)
	}
	data, err := renderStarterConfig(defaultInitOptions())
	if err != nil {
		t.Fatalf("renderStarterConfig: %v", err)
	}
	config := string(data)
	for _, expected := range []string{
		"[schemas.fetch]",
		"enabled = true",
		"cache = true",
		`timeout = "10s"`,
		`retries = 2`,
		"[parsing.json]",
		`mode = "auto"`,
		"[schemas.compile]",
		`timeout = "30s"`,
		`failure = "warn"`,
		`branchErrors = "best"`,
	} {
		if !strings.Contains(config, expected) {
			t.Fatalf("default config missing %q:\n%s", expected, config)
		}
	}
	if strings.Contains(config, "fetchRemote") || strings.Contains(config, "strict =") || strings.Contains(config, "[timeouts]") || strings.Contains(config, "include = [") {
		t.Fatalf("default config contains removed keys:\n%s", config)
	}
}

func TestInitOverwritePromptComesBeforeInterview(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".dollarlint.toml")
	writeFile(t, target, "# existing\n")

	opts := defaultInitOptions()
	prompter := fakeInitPrompter{confirms: []bool{false}}
	if err := confirmInitTarget(target, opts, true, &prompter); err == nil {
		t.Fatalf("expected existing config error")
	}
	if strings.Join(prompter.questions, "|") != "Overwrite existing .dollarlint.toml?" {
		t.Fatalf("questions = %+v", prompter.questions)
	}

	prompter = fakeInitPrompter{confirms: []bool{true}}
	if err := confirmInitTarget(target, opts, true, &prompter); err != nil {
		t.Fatalf("overwrite confirm: %v", err)
	}
	if strings.Join(prompter.questions, "|") != "Overwrite existing .dollarlint.toml?" {
		t.Fatalf("overwrite questions = %+v", prompter.questions)
	}
}

func TestNormalizeInitOptions(t *testing.T) {
	opts := defaultInitOptions()
	opts.catalogFailure = "skip"
	if err := normalizeInitOptions(&opts); err != nil {
		t.Fatalf("normalize skip: %v", err)
	}
	if opts.catalogFailure != "skip" {
		t.Fatalf("failure = %q", opts.catalogFailure)
	}
	opts.catalogFailure = "explode"
	if err := normalizeInitOptions(&opts); err == nil {
		t.Fatalf("expected invalid failure policy error")
	}
}

type fakeInitPrompter struct {
	confirms  []bool
	ints      []int
	failures  []string
	questions []string
}

func (p *fakeInitPrompter) Confirm(question string, defaultValue bool) (bool, error) {
	p.questions = append(p.questions, question)
	if len(p.confirms) == 0 {
		return defaultValue, nil
	}
	value := p.confirms[0]
	p.confirms = p.confirms[1:]
	return value, nil
}

func (p *fakeInitPrompter) NonNegativeInt(question string, defaultValue int) (int, error) {
	p.questions = append(p.questions, question)
	if len(p.ints) == 0 {
		return defaultValue, nil
	}
	value := p.ints[0]
	p.ints = p.ints[1:]
	return value, nil
}

func (p *fakeInitPrompter) CatalogFailure(defaultValue string) (string, error) {
	p.questions = append(p.questions, "catalogFailure")
	if len(p.failures) == 0 {
		return defaultValue, nil
	}
	value := p.failures[0]
	p.failures = p.failures[1:]
	return value, nil
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
