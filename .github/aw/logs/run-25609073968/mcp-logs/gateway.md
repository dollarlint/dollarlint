<details>
<summary>MCP Gateway</summary>

- ✓ **startup** MCPG Gateway version: v0.3.6
- ✓ **startup** Starting MCPG with config: stdin, listen: 0.0.0.0:8080, log-dir: /tmp/gh-aw/mcp-logs/
- ✓ **startup** Loaded 3 MCP server(s): [dollarlint-repo github safeoutputs]
- ✓ **startup** Guards sink server ID logging enrichment disabled (no sink server IDs configured)
- ✓ **startup** OpenTelemetry tracing disabled (no OTLP endpoint configured)
- ✗ **backend**
  ```
  MCP backend stderr output:
go: go.mod requires go >= 1.26.3 (running go 1.25.10; GOTOOLCHAIN=local)
  ```
- ✗ **backend**
  ```
  MCP backend connection/protocol error, command=docker
  ```
- ✗ **backend**
  ```
  MCP backend connection/protocol error, command=docker
  ```
- ✓ **backend**
  ```
  Successfully connected to MCP backend server, command=docker
  ```
- 🔍 rpc **github**→`tools/list`
- 🔍 rpc **safeoutputs**→`tools/list`
- 🔍 rpc **safeoutputs**←`resp` `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"description":"Create a GitHub discussion for announcements, Q\u0026A, reports, status updates, or community conversations. Use this for content that benefits from threaded replies, doesn't require task tracking, or serves as documentation. For actionable work items that need assignment and status tracking, use create_issue instead. CONSTRAINTS: Maximum 1 discussion(s) can be created. Title will be prefixed with \"Weekly real-world testing: \".","inputSchema":{"ad...`
- 🔍 rpc **github**←`resp` `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"annotations":{"readOnlyHint":true,"title":"Get commit details"},"description":"Get details for a commit from a GitHub repository","inputSchema":{"properties":{"include_diff":{"default":true,"description":"Whether to include file diffs and stats in the response. Default is true.","type":"boolean"},"owner":{"description":"Repository owner","type":"string"},"page":{"description":"Page number for pagination (min 1)","minimum":1,"type":"number"},"perPage":{"descriptio...`
- ✓ **startup** Starting MCPG in ROUTED mode on 0.0.0.0:8080
- ✓ **startup** Routes: /mcp/<server> where <server> is one of: [safeoutputs dollarlint-repo github]
- ✓ **startup** TLS not configured — listening on http://0.0.0.0:8080 (set --tls-cert/--tls-key to enable)
- ✗ **backend**
  ```
  MCP backend stderr output:
go: go.mod requires go >= 1.26.3 (running go 1.25.10; GOTOOLCHAIN=local)
  ```
- ✗ **backend**
  ```
  MCP backend connection/protocol error, command=docker
  ```
- ✗ **backend**
  ```
  MCP backend connection/protocol error, command=docker
  ```
- ✗ **backend**
  ```
  MCP backend stderr output:
go: go.mod requires go >= 1.26.3 (running go 1.25.10; GOTOOLCHAIN=local)
  ```
- ✗ **backend**
  ```
  MCP backend connection/protocol error, command=docker
  ```
- ✗ **backend**
  ```
  MCP backend connection/protocol error, command=docker
  ```
- ✗ **backend**
  ```
  MCP backend stderr output:
go: go.mod requires go >= 1.26.3 (running go 1.25.10; GOTOOLCHAIN=local)
  ```
- ✗ **backend**
  ```
  MCP backend connection/protocol error, command=docker
  ```
- ✗ **backend**
  ```
  MCP backend connection/protocol error, command=docker
  ```
- 🔍 rpc **safeoutputs**→`tools/call` `missing_tool`
  
  ```json
  {"params":{"arguments":{"reason":"security"},"name":"missing_tool"}}
  ```
- 🔍 rpc **safeoutputs**←`resp`
  
  ```json
  {"id":1,"result":{"content":[{"text":"{\"result\":\"success\"}","type":"text"}]}}
  ```
- 🔍 rpc **safeoutputs**→`tools/call` `{"jsonrpc":"2.0","method":"tools/call","params":{"arguments":{"body":"\n### Run Summary\n\nThis weekly real-world corpus run could not be completed. The run was planned for 5 fresh repositories from the default pool (gitea, prettier, fastapi, neovim, pnpm — none previously tested). No results were fabricated.\n\n### Blocker\n\nTwo independent blockers prevented any validation from running:\n\n1. **dollarlint-repo MCP server unavailable.** The tools file at `/home/runner/work/_temp/gh-aw/mcp-cli/tools/doll...`
- 🔍 rpc **safeoutputs**←`resp`
  
  ```json
  {"id":1,"result":{"content":[{"text":"{\"result\":\"success\"}","type":"text"}]}}
  ```
