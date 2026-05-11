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

description: Run DollarLint Agentic Product Testing against a real-world corpus and publish a GitHub Discussion summary.
labels: [automation, agentic-product-testing]

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

tools:
  startup-timeout: 300
  timeout: 1800
  edit:
  bash:
    - date
    - jq
    - printf
    - git status
    - git diff
    - git diff --stat
    - sed
    - cat
    - head
    - tail
    - wc
    # The repo MCP CLI wrapper is broad here, but the server exposes only
    # real_world_* tools via DOLLARLINT_MCP_TOOLS below.
    - dollarlint-repo:*
  github:
    toolsets: [repos]

mcp-servers:
  dollarlint-repo:
    container: golang
    version: "1.26.3@sha256:2981696eed011d747340d7252620932677929cce7d2d539602f56a8d7e9b660b"
    entrypoint: /bin/sh
    entrypointArgs:
      - -lc
      - exec ${GITHUB_WORKSPACE}/tools/repo-mcp/start-gh-aw.sh
    mounts:
      - "${GITHUB_WORKSPACE}:${GITHUB_WORKSPACE}:rw"
    env:
      GITHUB_WORKSPACE: "${GITHUB_WORKSPACE}"
      DOLLARLINT_REPO_ROOT: "${GITHUB_WORKSPACE}"
      DOLLARLINT_MCP_TOOLS: "real_world_*"
      PATH: /usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
      GOCACHE: /tmp/go-cache
      GOMODCACHE: /tmp/go-mod-cache
    allowed:
      # The gateway allowlist stays broad so the repo MCP can evolve without
      # editing this workflow; DOLLARLINT_MCP_TOOLS is the server-side filter.
      - "*"

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
  max-patch-size: 8192
  mentions:
    allow-team-members: false
    allow-context: false
    allowed: [agorischek]
    max: 2
  allowed-github-references: []
  create-discussion:
    title-prefix: "Agentic Product Testing: "
    category: agentic-product-testing
    max: 1
    fallback-to-issue: true
    close-older-discussions: true
    expires: 30d
  create-pull-request:
    title-prefix: "Update Agentic Product Testing results: "
    max: 1
    labels: [agentic-workflows]
    if-no-changes: error
    allowed-files:
      - reports/agentic-product-testing/**
    protected-files: fallback-to-issue
    draft: false

timeout-minutes: 90
---

# Agentic Product Testing

Run an Agentic Product Testing sweep for DollarLint, record the structured result, and open one GitHub Discussion with the results.

Use the `dollarlint-repo` MCP server as the workflow source of truth. Start with `real_world_start_testing`, then follow the `nextStep` guidance returned by the `real_world_*` tools until the run is recorded. Prefer the MCP wizard over shell commands and hand-written reports. If the MCP capabilities are missing or stale, stop and publish a blocker summary instead of improvising.

Manual dispatch inputs for this run:
- max_repos: `${{ github.event.inputs.max_repos }}`
- candidate_repos: `${{ github.event.inputs.candidate_repos }}`

Before calling `real_world_start_testing`, derive the repository plan from those literal inputs. Treat a missing or empty `max_repos` as `10`. If `candidate_repos` is a non-empty JSON array, parse it and pass at most `max_repos` entries exactly to `real_world_start_testing.repositories`; do not substitute different repositories unless MCP history reports duplicates and `allowPreviouslyTested` is false. If `candidate_repos` is empty, choose up to `max_repos` diverse, well-known public repositories with conventional config files and let `real_world_start_testing` check them against MCP history; do not fetch the full tested-repository history first. If duplicates are reported, use the returned `candidateSetID` and call `real_world_update_candidates` with a small `diff.replace`, `diff.remove`, or `diff.add` instead of resubmitting the full repository list. When the candidate set is ready, prefer passing only `candidateSetID` and `expectedCount` to `real_world_prepare_corpus`.

The workflow pre-builds `bin/dollarlint`; when the MCP flow asks for validation arguments, use `build: false` so validation uses the prebuilt CLI. Keep long-running prep or validation MCP calls open for progress notifications, and do not poll with shell sleep loops.

Dependency prep is controlled structurally, not by judgment alone: this workflow does not expose package-manager commands through `tools.bash`, the agent job has read-only GitHub permissions, and `create_pull_request` is restricted to `reports/agentic-product-testing/**`. Use only the `real_world_*` MCP tools for cloning, inspection, validation, and result recording. Treat MCP `suggestedCommands` as audit guidance for dependency-prep notes; do not execute package-manager install/fetch commands from the agent shell. If schema fidelity would require dependency materialization that the MCP server cannot perform safely, record dependency prep as skipped or needs-review with the reason.

Durable repository memory must be written through `real_world_record_result` and the structured JSON run directory it manages under `reports/agentic-product-testing/<run-id>/`. Do not create Markdown report files or a shared summary index. Product recommendations are mandatory: include a `high`, `med`, or `low` strength with rationale, or record an explicit no-change recommendation when DollarLint behaved reasonably. If `real_world_record_result` changes repository files, `create_pull_request` is mandatory because merging that PR is how the repo remembers tested repositories. The PR body must say that it should be merged in order to retain the results in Agentic Product Testing memory and must include this workflow run URL: `${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}`.

After recording, create exactly one GitHub Discussion through the configured safe output in the `Agentic Product Testing` category. Keep it concise: result counts, tested repositories, notable findings, product recommendations with strength labels, persisted artifact path, DollarLint commit, and workflow run URL. Include `@agorischek` near the top of the Discussion body so the owner is notified. Put verbose examples or raw warnings inside `<details>` blocks. The Discussion body must include a "Durable memory PR" section saying that a companion PR will be opened and that the PR should be merged in order to retain the results for future sweeps. If the Discussion falls back to an issue, say that the intended destination was a Discussion.

The workflow may expose safe outputs either as direct tools or through a `safeoutputs` CLI wrapper. Use the available safe-output interface, but do not stop after committing locally. If using the CLI wrapper, send inline JSON on stdin with `printf '%s' '<json>' | safeoutputs create_pull_request .` and `printf '%s' '<json>' | safeoutputs create_discussion .`; do not rely on temporary payload files.

After requesting both `create_pull_request` and `create_discussion`, no extra linking tool call is needed. A deterministic follow-up workflow cross-links the final PR and Discussion URLs after the run completes.

Do not fabricate results. If cloning, preparation, validation, triage, or recording blocks before a meaningful sweep completes, publish a short blocker summary with any partial artifacts and the next concrete fix.

Final response: choose exactly one outcome. Either recommend product changes to consider with strength and rationale grounded in the MCP record, or state that the product behaved reasonably and no product change is recommended. Do not finish with only raw counts or run mechanics.
