package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
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
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"dev"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"validate","arguments":{"include":["bad.json"]}}}`,
		"",
	}, "\n")
	var stdout, stderr bytes.Buffer
	if code := ExecuteWithIO([]string{"serve", "mcp"}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("serve mcp exit = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	responses := readMCPResponses(t, stdout.String())
	if len(responses) != 3 {
		t.Fatalf("response count = %d output=%s", len(responses), stdout.String())
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
	summary := result["summary"].(map[string]any)
	issues := summary["issues"].(map[string]any)
	if issues["total"] != float64(1) {
		t.Fatalf("summary = %+v", summary)
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
