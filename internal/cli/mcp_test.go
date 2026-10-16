package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServeMCPValidateTool(t *testing.T) {
	dir := t.TempDir()
	writeMCPTestFile(t, filepath.Join(dir, "schema.json"), `{"type":"object","required":["name"],"properties":{"$schema":{"type":"string"},"name":{"type":"string"}}}`)
	writeMCPTestFile(t, filepath.Join(dir, "bad.json"), `{"$schema":"./schema.json"}`)
	t.Chdir(dir)
	responses := runMCPTestServer(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"dev"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"validate","arguments":{"include":["bad.json"]}}}`,
	)
	if len(responses) != 3 {
		t.Fatalf("response count = %d", len(responses))
	}
	initializeResponse := responseWithID(t, responses, 1)
	if initializeResponse["error"] != nil {
		t.Fatalf("initialize error = %+v", initializeResponse["error"])
	}
	initResult := initializeResponse["result"].(map[string]any)
	if initResult["protocolVersion"] != "2025-11-25" {
		t.Fatalf("initialize protocol = %+v", initResult)
	}
	listResult := responseWithID(t, responses, 2)["result"].(map[string]any)
	tools := listResult["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "validate" {
		t.Fatalf("tools/list result = %+v", listResult)
	}
	inputSchema := tools[0].(map[string]any)["inputSchema"].(map[string]any)
	properties := inputSchema["properties"].(map[string]any)
	if len(properties) != 1 || properties["include"] == nil {
		t.Fatalf("input schema properties = %+v", properties)
	}
	callResult := responseWithID(t, responses, 3)["result"].(map[string]any)
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
	responses := runMCPTestServer(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"dev"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"missing","arguments":{}}}`,
	)
	if len(responses) != 2 {
		t.Fatalf("response count = %d", len(responses))
	}
	responseError := responseWithID(t, responses, 2)["error"].(map[string]any)
	if responseError["code"] != float64(-32602) || !strings.Contains(responseError["message"].(string), "tool 'missing' not found") {
		t.Fatalf("error = %+v", responseError)
	}
}

func runMCPTestServer(t *testing.T, messages ...string) []map[string]any {
	t.Helper()
	inputReader, inputWriter := io.Pipe()
	stdout := &mcpTestOutput{writes: make(chan struct{}, len(messages))}
	done := make(chan int, 1)
	go func() {
		done <- ExecuteWithIO([]string{"serve", "mcp"}, inputReader, stdout, io.Discard)
	}()
	finished := false
	defer func() {
		_ = inputWriter.Close()
		if !finished {
			<-done
		}
	}()

	for _, message := range messages {
		if _, err := io.WriteString(inputWriter, message+"\n"); err != nil {
			t.Fatalf("write MCP request: %v", err)
		}
		var request struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal([]byte(message), &request); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		if len(request.ID) == 0 {
			continue
		}
		select {
		case <-stdout.writes:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for MCP response; output=%s", stdout.String())
		}
	}
	if err := inputWriter.Close(); err != nil {
		t.Fatalf("close MCP input: %v", err)
	}
	code := <-done
	finished = true
	if code != 0 {
		t.Fatalf("serve mcp exit = %d stdout=%s", code, stdout.String())
	}
	return readMCPResponses(t, stdout.String())
}

type mcpTestOutput struct {
	mu     sync.Mutex
	output bytes.Buffer
	writes chan struct{}
}

func (w *mcpTestOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	n, err := w.output.Write(data)
	w.mu.Unlock()
	if err == nil {
		w.writes <- struct{}{}
	}
	return n, err
}

func (w *mcpTestOutput) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.output.String()
}

func responseWithID(t *testing.T, responses []map[string]any, id float64) map[string]any {
	t.Helper()
	for _, response := range responses {
		if response["id"] == id {
			return response
		}
	}
	t.Fatalf("response with id %v not found in %+v", id, responses)
	return nil
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
