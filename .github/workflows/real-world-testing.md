---
on:
  workflow_dispatch:
    inputs:
      candidate_repos:
        description: Optional JSON array of repositories with name, ecosystem, and cloneURL. Leave empty to choose a fresh diverse corpus.
        required: false
        type: string
      max_repos:
        description: Maximum number of fresh repositories to test.
        required: false
        default: "10"
        type: string
  schedule: weekly on monday

description: Run DollarLint against a real-world corpus and publish a GitHub Discussion summary.
labels: [automation, real-world-testing]

permissions:
  contents: read

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
    - dollarlint-repo:*
  github:
    toolsets: [repos]

mcp-servers:
  dollarlint-repo:
    type: stdio
    container: golang:1.26.3
    entrypoint: /bin/sh
    entrypointArgs:
      - -lc
      - export PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; export DOLLARLINT_REPO_ROOT=$GITHUB_WORKSPACE; git config --global --add safe.directory $GITHUB_WORKSPACE || true; cd $GITHUB_WORKSPACE && go run ./tools/repo-mcp
    mounts:
      - "${GITHUB_WORKSPACE}:${GITHUB_WORKSPACE}:rw"
    env:
      GITHUB_WORKSPACE: "${GITHUB_WORKSPACE}"
      DOLLARLINT_REPO_ROOT: "${GITHUB_WORKSPACE}"
      PATH: /usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
      GOCACHE: /tmp/go-cache
      GOMODCACHE: /tmp/go-mod-cache
    allowed:
      - real_world_start_testing
      - real_world_history
      - real_world_artifact_query
      - real_world_prepare_corpus
      - real_world_next_prepared_repo
      - real_world_prepare_status
      - real_world_cancel_prepare
      - real_world_inspect_corpus
      - real_world_start_validation
      - real_world_next_validation_result
      - real_world_record_validation_feedback
      - real_world_validation_status
      - real_world_finish_validation
      - real_world_cancel_validation
      - real_world_triage_output
      - real_world_recommendation_backlog
      - real_world_record_result

env:
  CANDIDATE_REPOS: ${{ inputs.candidate_repos || '' }}
  MAX_REPOS: ${{ inputs.max_repos || '10' }}

pre-agent-steps:
  - name: Build DollarLint CLI
    run: |
      mkdir -p bin
      go build -o bin/dollarlint ./cmd/dollarlint
      bin/dollarlint --version

safe-outputs:
  mentions: false
  allowed-github-references: []
  create-discussion:
    title-prefix: "Real-world testing: "
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

# Real-World Testing

Run a real-world DollarLint sweep for up to `MAX_REPOS` fresh public repositories, record the structured result, and open one GitHub Discussion with the results.

Use the `dollarlint-repo` MCP server as the workflow source of truth. Start with `real_world_start_testing`, then follow the `nextStep` guidance returned by the `real_world_*` tools until the run is recorded. Prefer the MCP wizard over shell commands and hand-written reports. If the MCP capabilities are missing or stale, stop and publish a blocker summary instead of improvising.

If `CANDIDATE_REPOS` is a non-empty JSON array, use it as the candidate pool. Otherwise choose up to `MAX_REPOS` diverse, well-known public repositories with conventional config files, skipping repositories already in MCP history unless the manual input explicitly asks for reruns.

The workflow pre-builds `bin/dollarlint`; when the MCP flow asks for validation arguments, use `build: false` so validation uses the prebuilt CLI. Keep long-running prep or validation MCP calls open for progress notifications, and do not poll with shell sleep loops. Never run dependency lifecycle scripts, postinstall hooks, package-manager plugins, or repository install scripts.

Durable repository memory must be written through `real_world_record_result` and the structured JSON files it manages. Do not create Markdown report files. Product recommendations are mandatory: include a `high`, `med`, or `low` strength with rationale, or record an explicit no-change recommendation when DollarLint behaved reasonably. If the recorded result changes repository files, request one pull request through the configured safe output.

After recording, create exactly one GitHub Discussion through the configured safe output. Keep it concise: result counts, tested repositories, notable findings, product recommendations with strength labels, persisted artifact path, DollarLint commit, and workflow run URL. Put verbose examples or raw warnings inside `<details>` blocks. If the Discussion falls back to an issue, say that the intended destination was a Discussion.

Do not fabricate results. If cloning, preparation, validation, triage, or recording blocks before a meaningful sweep completes, publish a short blocker summary with any partial artifacts and the next concrete fix.

Final response: choose exactly one outcome. Either recommend product changes to consider with strength and rationale grounded in the MCP record, or state that the product behaved reasonably and no product change is recommended. Do not finish with only raw counts or run mechanics.
