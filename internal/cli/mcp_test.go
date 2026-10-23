package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeMCPValidateTool(t *testing.T) {
	dir := t.TempDir()
	writeMCPTestFile(t, filepath.Join(dir, "schema.json"), `{"type":"object","required":["name"],"properties":{"$schema":{"type":"string"},"name":{"type":"string"}}}`)
	writeMCPTestFile(t, filepath.Join(dir, "bad.json"), `{"$schema":"./schema.json"}`)
	t.Chdir(dir)
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	defer inputWriter.Close()
	defer outputReader.Close()
	type serveResult struct {
		code   int
		stderr string
	}
	serveDone := make(chan serveResult, 1)
	go func() {
		var stderr bytes.Buffer
		code := ExecuteWithIO([]string{"serve", "mcp"}, inputReader, outputWriter, &stderr)
		_ = outputWriter.Close()
		serveDone <- serveResult{code: code, stderr: stderr.String()}
	}()
	responseReader := bufio.NewReader(outputReader)
	var responses []map[string]any
	sendRequest := func(request string) map[string]any {
		t.Helper()
		if _, err := io.WriteString(inputWriter, request+"\n"); err != nil {
			t.Fatalf("write request: %v", err)
		}
		line, err := responseReader.ReadString('\n')
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		responses := readMCPResponses(t, line)
		if len(responses) != 1 {
			t.Fatalf("response count = %d output=%s", len(responses), line)
		}
		return responses[0]
	}
	responses = []map[string]any{
		sendRequest(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"dev"}}}`),
	}
	if _, err := io.WriteString(inputWriter, "{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n"); err != nil {
		t.Fatalf("write initialized notification: %v", err)
	}
	responses = append(responses,
		sendRequest(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`),
		sendRequest(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"validate","arguments":{"include":["bad.json"]}}}`),
	)
	if err := inputWriter.Close(); err != nil {
		t.Fatalf("close request stream: %v", err)
	}
	execution := <-serveDone
	if execution.code != 0 {
		t.Fatalf("serve mcp exit = %d stderr=%s", execution.code, execution.stderr)
	}
	if len(responses) != 3 {
		t.Fatalf("response count = %d", len(responses))
	}
	if responses[0]["error"] != nil {
		t.Fatalf("initialize error = %+v", responses[0]["error"])
	}
	initResult := responses[0]["result"].(map[string]any)
	if initResult["protocolVersion"] != "2025-11-25" {
		t.Fatalf("initialize protocol = %+v", initResult)
	}
	listResult := responses[1]["result"].(map[string]any)
	tools := listResult["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "validate" {
		t.Fatalf("tools/list result = %+v", listResult)
	}
	inputSchema := tools[0].(map[string]any)["inputSchema"].(map[string]any)
	properties := inputSchema["properties"].(map[string]any)
	if len(properties) != 1 || properties["include"] == nil {
		t.Fatalf("input schema properties = %+v", properties)
	}
	callResult := responses[2]["result"].(map[string]any)
	if callResult["isError"] == true {
		t.Fatalf("validate tool returned execution error = %+v", callResult)
	}
	structured := callResult["structuredContent"].(map[string]any)
	if structured["ok"] != false || structured["message"] != "validation issues found" {
		t.Fatalf("structured content = %+v", structured)
	}
	result := structured["result"].(map[string]any)
	if result["formatVersion"] != float64(1) {
		t.Fatalf("result format version = %+v", result)
	}
	summary := result["summary"].(map[string]any)
	issues := summary["issues"].(map[string]any)
	if issues["total"] != float64(1) {
		t.Fatalf("summary = %+v", summary)
	}
	resultIssues := result["issues"].([]any)
	firstIssue := resultIssues[0].(map[string]any)
	if firstIssue["path"] != "bad.json" || firstIssue["category"] != "validation" || firstIssue["line"] == nil {
		t.Fatalf("result issues = %+v", resultIssues)
	}
	content := callResult["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, `"ok":false`) || !strings.Contains(text, `"total":1`) {
		t.Fatalf("content text = %s", text)
	}
}

func TestServeMCPUnknownTool(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"missing","arguments":{}}}` + "\n"
	var stdout, stderr bytes.Buffer
	if code := ExecuteWithIO([]string{"serve", "mcp"}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("serve mcp exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	responses := readMCPResponses(t, stdout.String())
	if len(responses) != 1 {
		t.Fatalf("response count = %d output=%s", len(responses), stdout.String())
	}
	responseError := responses[0]["error"].(map[string]any)
	if responseError["code"] != float64(-32602) || !strings.Contains(responseError["message"].(string), "tool 'missing' not found") {
		t.Fatalf("error = %+v", responseError)
	}
}

func readMCPResponses(t *testing.T, output string) []map[string]any {
	t.Helper()
	var responses []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		var response map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			t.Fatalf("decode response %q: %v", scanner.Text(), err)
		}
		responses = append(responses, response)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan responses: %v", err)
	}
	return responses
}

func writeMCPTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
