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

description: Run DollarLint against a fresh real-world corpus and publish a GitHub Discussion summary.
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
      - real_world_record_result

env:
  CANDIDATE_REPOS: ${{ inputs.candidate_repos || '' }}
  MAX_REPOS: ${{ inputs.max_repos || '5' }}

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

Run DollarLint against a fresh, bounded set of public repositories, persist the structured MCP result, and publish a GitHub Discussion summary for this workflow run.

## Workflow

1. Use the `dollarlint-repo` MCP server first.
   - Call `real_world_start_testing` to inspect the `reports/real-world-results.json` index and split entry files under `reports/real-world-results/`.
   - Follow the `nextStep` guidance returned by each real-world MCP tool.
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
   - Call `real_world_prepare_corpus` with `clone: true`. This starts managed clone and dependency-prep inspection jobs in the background and returns a manifest immediately.
   - If it reports duplicates, choose replacements unless this was an intentional manual rerun.
   - Do not wait for the full corpus to clone before validation. Use the returned `nextStep` and call `real_world_start_validation` with the prepared `corpusDir`, `cacheDir`, `outputArtifact`, `manifestPath`, `build: false`, and `waitForFirstResult: true`; validation will wait inside the tool for repositories that are still being prepared.
   - Use dependency-prep first-pass notes from the managed preparation manifest and `real_world_finish_validation`; call `real_world_next_prepared_repo` only if you specifically need to inspect clone/dependency-prep details before validation reaches that repo, and call `real_world_inspect_corpus` only if you need to re-run the scan.
   - Never run dependency lifecycle scripts, postinstall hooks, package-manager plugins, or repository install scripts during dependency prep.
   - If any dependency-prep entry has `status: needs-review`, run a bounded prep command only when lifecycle scripts are disabled and it is needed for local `$schema` fidelity. Otherwise replace it with an explicit skipped/not-needed note.
   - The workflow pre-builds `bin/dollarlint` in a deterministic pre-agent step.
   - Do not use shell sleep loops to monitor corpus prep or validation. For long work, keep `real_world_start_validation`, `real_world_next_validation_result`, or `real_world_next_prepared_repo` open; those tool calls send progress notifications and return per-repository results as they complete.
   - After each per-repository result, review the product signal from that repository while the remaining jobs continue. Record `validationFeedback` for that repository in the next `real_world_next_validation_result` call, or call `real_world_record_validation_feedback` if you need to record feedback without waiting.
   - Continue calling `real_world_next_validation_result` with feedback for the previously delivered result until `nextStep` asks for `real_world_finish_validation`.
   - Call `real_world_finish_validation` to merge completed per-repository artifacts into the standard JSON `outputArtifact` before triage.
   - Nonzero validation exit code 1 is expected when real issues are found. Treat crashes, missing output JSON, hangs, and unexplained warnings as higher-severity product signals.

4. Triage the output.
   - Call `real_world_triage_output` with the prepared `corpusDir`, `cacheDir`, `outputArtifact`, `manifestPath` when available, validation command, repositories, dependency prep entries, and `validationFeedback` from `real_world_finish_validation`.
   - Let the MCP tool sanity-check output counts and group parsing, validation, schema, coverage, warning, crash/performance, and output-contract signals by repository.
   - If `real_world_triage_output` returns an error, resolve the output/schema mismatch or record a blocker instead of proceeding from a hand-written Markdown report.
   - Use `draftRecord` as the starting point for the structured result. Adjust it only with evidence from the JSON artifact, dependency prep notes, validation feedback, or repository context.
   - Product recommendations must include a strength label of `high`, `med`, or `low`, based on frequency, severity, reproducibility, and expected user impact. If there is no genuine product change to consider, use an explicit no-change recommendation in the record.

5. Persist repository memory.
   - Call `real_world_record_result` with the title, corpus, cache directory, command, output artifact, repositories, dependency prep entries, validation feedback, findings, `productRecommendations` objects with `high`/`med`/`low` strength and rationale, product changes/decisions in `productDecisions`, and follow-up notes from the triage tool's `draftRecord`.
   - `real_world_record_result` automatically copies the raw DollarLint JSON output into `reports/real-world-artifacts/`, stores the repo-relative path as `persistedOutputArtifact`, and cleans managed temp corpus/cache dirs after recording succeeds; use that durable artifact for later per-file triage.
   - Do not create or update Markdown report files in the repository. Durable repository memory belongs in the MCP structured result.
   - If files changed, request a pull request through the configured `create-pull-request` safe output. Keep the PR title concise and mention the structured result entry.

6. Publish the GitHub Agentic Workflow summary.
   - This Discussion is only for this GitHub Agentic Workflow run. The durable source of truth must already be the MCP structured result from step 5.
   - Request one GitHub Discussion through the configured `create-discussion` safe output.
   - Title: use the run date and a short result summary, for example `2026-05-09 - 5 repo corpus`.
   - Body style:
     - Use `###` and `####` headings only.
     - Keep the summary, result counts, notable findings, and product recommendations with `high`/`med`/`low` strength labels visible.
     - Put verbose per-file details and raw warnings inside `<details>` blocks.
     - Include the tested repository table, DollarLint commit, persisted output artifact path, temp output artifact path, and workflow run URL.
   - If Discussion creation is unavailable, the safe output may fall back to an issue; mention that the intended destination was a Discussion.

7. Final response.
   - The final message back to the user must choose exactly one outcome:
     - Recommend product changes to consider, with strength and rationale grounded in the MCP triage/record result.
     - State that the product behaved reasonably, with a brief explanation of why no product change is recommended.
   - Do not finish with only raw counts, run mechanics, or a generic summary.

Do not fabricate results. If cloning or validation fails before a meaningful corpus run, request a short GitHub Agentic Workflow summary with the blocker, partial artifacts, and the next concrete fix.
