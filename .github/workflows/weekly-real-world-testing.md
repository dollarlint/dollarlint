---
on:
  workflow_dispatch:
    inputs:
      candidate_repos:
        description: Optional JSON array of repositories with name, ecosystem, and cloneURL. Leave empty to use the default pool.
        required: false
        type: string
      max_repos:
        description: Maximum number of fresh repositories to test.
        required: false
        default: "5"
        type: string
  schedule: weekly on monday

description: Run DollarLint against a fresh real-world corpus and publish a weekly report.
labels: [automation, real-world-testing]

permissions:
  contents: read
  discussions: read
  issues: read
  pull-requests: read
  actions: read

engine: copilot

runtimes:
  go:
    version: "1.26.3"

network:
  allowed:
    - defaults
    - github
    - go
    - node
    - python
    - ruby
    - rust
    - java
    - dotnet
    - php
    - dart
    - terraform
    - linux-distros
    - "openapi.vercel.sh"
    - "changie.dev"

tools:
  startup-timeout: 300
  timeout: 1800
  edit:
  bash:
    - date
    - jq
    - git status
    - git diff
    - git diff --stat
    - sed
    - cat
    - head
    - tail
    - wc
  github:
    toolsets: [repos]

mcp-servers:
  dollarlint-repo:
    type: stdio
    container: golang:1.26.3
    entrypoint: /bin/sh
    entrypointArgs:
      - -lc
      - export PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; cd $GITHUB_WORKSPACE && go run ./tools/repo-mcp
    mounts:
      - "${GITHUB_WORKSPACE}:${GITHUB_WORKSPACE}:rw"
    env:
      GITHUB_WORKSPACE: "${GITHUB_WORKSPACE}"
      PATH: /usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
      GOCACHE: /tmp/go-cache
      GOMODCACHE: /tmp/go-mod-cache
    allowed:
      - real_world_history
      - real_world_prepare_corpus
      - real_world_run_corpus
      - real_world_record_result

env:
  CANDIDATE_REPOS: ${{ inputs.candidate_repos || '' }}
  MAX_REPOS: ${{ inputs.max_repos || '5' }}

safe-outputs:
  mentions: false
  allowed-github-references: []
  create-discussion:
    title-prefix: "Weekly real-world testing: "
    max: 1
    fallback-to-issue: true
    close-older-discussions: true
    expires: 30
  create-pull-request:
    title-prefix: "Update real-world test results: "
    max: 1
    protected-files: fallback-to-issue
    draft: false

timeout-minutes: 90
---

# Weekly Real-World Testing

Run DollarLint against a fresh, bounded set of public repositories, persist the structured result, and publish a Discussion report.

## Workflow

1. Use the `dollarlint-repo` MCP server first.
   - Call `real_world_history` to inspect `reports/real-world-results.json`.
   - Use it to avoid repositories that have already been tested unless the manual input explicitly asks for a rerun.

2. Choose the corpus.
   - If `CANDIDATE_REPOS` is a non-empty JSON array, parse it as the candidate pool. Each item should have `name`, `ecosystem`, and `cloneURL`.
   - Otherwise use this default pool, skipping anything already in history:

```json
[
  {"name":"gitea","ecosystem":"Go/TypeScript","cloneURL":"https://github.com/go-gitea/gitea.git"},
  {"name":"vite","ecosystem":"TypeScript/Node","cloneURL":"https://github.com/vitejs/vite.git"},
  {"name":"prettier","ecosystem":"JavaScript/Node","cloneURL":"https://github.com/prettier/prettier.git"},
  {"name":"fastapi","ecosystem":"Python","cloneURL":"https://github.com/fastapi/fastapi.git"},
  {"name":"ansible","ecosystem":"Python/YAML","cloneURL":"https://github.com/ansible/ansible.git"},
  {"name":"neovim","ecosystem":"C/Lua","cloneURL":"https://github.com/neovim/neovim.git"},
  {"name":"deno","ecosystem":"Rust/TypeScript","cloneURL":"https://github.com/denoland/deno.git"},
  {"name":"grafana","ecosystem":"Go/TypeScript","cloneURL":"https://github.com/grafana/grafana.git"},
  {"name":"pnpm","ecosystem":"TypeScript/Node","cloneURL":"https://github.com/pnpm/pnpm.git"},
  {"name":"huggingface-transformers","ecosystem":"Python","cloneURL":"https://github.com/huggingface/transformers.git"},
  {"name":"node","ecosystem":"C++/JavaScript","cloneURL":"https://github.com/nodejs/node.git"},
  {"name":"kibana","ecosystem":"TypeScript/Node","cloneURL":"https://github.com/elastic/kibana.git"}
]
```

   - Select up to `MAX_REPOS` repositories. Prefer a small, diverse set over a huge one. Default to 5.
   - If the default pool is exhausted, pick other well-known public repositories with distinct ecosystems and conventional config files.

3. Prepare and run.
   - Call `real_world_prepare_corpus` with `clone: true`.
   - If it reports duplicates, choose replacements unless this was an intentional manual rerun.
   - Call `real_world_run_corpus` with the prepared `corpusDir`, `cacheDir`, and `outputArtifact`.
   - Nonzero validation exit code 1 is expected when real issues are found. Treat crashes, missing output JSON, hangs, and unexplained warnings as higher-severity product signals.

4. Triage the output.
   - Inspect the DollarLint JSON output artifact with `jq`.
   - Separate parsing, validation, schema, coverage, warnings, crashes, performance, and output-contract findings.
   - Decide whether each finding is a real third-party issue, expected test fixture data, SchemaStore mismatch, parser compatibility gap, confusing output/reporting problem, or DollarLint bug.
   - Summarize product recommendations before finishing the run. Each recommendation must include a strength label of `high`, `med`, or `low`, based on frequency, severity, reproducibility, and expected user impact.

5. Persist repository memory.
   - Call `real_world_record_result` with the title, corpus, cache directory, command, output artifact, repositories, findings, product recommendations with strength labels in `productDecisions`, and follow-up notes.
   - Also append a factual narrative entry to `reports/REAL-WORLD-TESTS.md` using the existing report shape.
   - If files changed, request a pull request through the configured `create-pull-request` safe output. Keep the PR title concise and mention the structured result file.

6. Publish the report.
   - Request one GitHub Discussion through the configured `create-discussion` safe output.
   - Title: use the run date and a short result summary, for example `2026-05-09 - 5 repo corpus`.
   - Body style:
     - Use `###` and `####` headings only.
     - Keep the summary, result counts, notable findings, and product recommendations with `high`/`med`/`low` strength labels visible.
     - Put verbose per-file details and raw warnings inside `<details>` blocks.
     - Include the tested repository table, DollarLint commit, output artifact path, and workflow run URL.
   - If Discussion creation is unavailable, the safe output may fall back to an issue; mention that the intended destination was a Discussion.

Do not fabricate results. If cloning or validation fails before a meaningful corpus run, publish a short failure report with the blocker, partial artifacts, and the next concrete fix.
