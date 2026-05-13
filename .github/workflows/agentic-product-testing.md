---
on:
  workflow_dispatch:
    inputs:
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
    # non-remote real_world_* tools via DOLLARLINT_MCP_TOOLS below.
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
      DOLLARLINT_MCP_TOOLS: "real_world_* !real_world_remote_*"
      PATH: /usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
      GOCACHE: /tmp/go-cache
      GOMODCACHE: /tmp/go-mod-cache
    allowed:
      # The gateway allowlist stays broad so the repo MCP can evolve without
      # editing this workflow; DOLLARLINT_MCP_TOOLS is the server-side filter.
      - "*"

env:
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

post-steps:
  - name: Fail incomplete Agentic Product Testing run
    if: always()
    run: |
      if [ -f "${GH_AW_SAFE_OUTPUTS}" ] && jq -e 'select(.type == "report_incomplete")' "${GH_AW_SAFE_OUTPUTS}" >/dev/null; then
        echo "::error::Agent reported the Agentic Product Testing run incomplete."
        exit 1
      fi

timeout-minutes: 90
---

# Agentic Product Testing

Run real-world testing for `${{ env.MAX_REPOS }}` repos.

Use `real_world_start_testing` and follow MCP `nextStep` guidance until recorded.
If files changed, create the results PR.
Open one Discussion with counts, linked repos, findings, recommendations, artifact path, commit, run URL, and `@agorischek`.
If blocked, publish a concise blocker.
