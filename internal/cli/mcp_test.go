package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServeMCPValidateTool(t *testing.T) {
	dir := t.TempDir()
	writeMCPTestFile(t, filepath.Join(dir, "schema.json"), `{"type":"object","required":["name"],"properties":{"$schema":{"type":"string"},"name":{"type":"string"}}}`)
	writeMCPTestFile(t, filepath.Join(dir, "bad.json"), `{"$schema":"./schema.json"}`)
	t.Chdir(dir)
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"dev"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"validate","arguments":{"include":["bad.json"]}}}`,
	}
	responses := exchangeMCPRequests(t, requests)
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
	responses := exchangeMCPRequests(t, []string{`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"missing","arguments":{}}}`})
	responseError := responses[0]["error"].(map[string]any)
	if responseError["code"] != float64(-32602) || !strings.Contains(responseError["message"].(string), "tool 'missing' not found") {
		t.Fatalf("error = %+v", responseError)
	}
}

// Keep stdin open until each request has received its response. The server may
// dispatch requests asynchronously; EOF is shutdown, not a request to flush work.
func exchangeMCPRequests(t *testing.T, requests []string) []map[string]any {
	t.Helper()
	stdin, input := io.Pipe()
	output, stdout := io.Pipe()
	var stderr bytes.Buffer
	done := make(chan int, 1)
	timer := time.AfterFunc(10*time.Second, func() {
		_ = input.CloseWithError(fmt.Errorf("MCP exchange timed out"))
		_ = output.CloseWithError(fmt.Errorf("MCP exchange timed out"))
	})
	defer timer.Stop()
	go func() {
		code := ExecuteWithIO([]string{"serve", "mcp"}, stdin, stdout, &stderr)
		_ = stdin.Close()
		_ = stdout.Close()
		done <- code
	}()
	defer func() {
		_ = input.Close()
		_ = output.Close()
		select {
		case code := <-done:
			if code != 0 {
				t.Errorf("serve mcp exit = %d stderr=%s", code, stderr.String())
			}
		case <-time.After(10 * time.Second):
			t.Error("MCP server did not stop after stdin closed")
		}
	}()
	scanner := bufio.NewScanner(output)
	var responses []map[string]any
	for _, request := range requests {
		if _, err := fmt.Fprintln(input, request); err != nil {
			t.Fatalf("write request: %v", err)
		}
		var message map[string]any
		if err := json.Unmarshal([]byte(request), &message); err != nil {
			t.Fatal(err)
		}
		if message["id"] == nil {
			continue
		}
		if !scanner.Scan() {
			t.Fatalf("missing response for id %v: %v", message["id"], scanner.Err())
		}
		var response map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response["id"] != message["id"] {
			t.Fatalf("response id = %v, want %v", response["id"], message["id"])
		}
		responses = append(responses, response)
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
