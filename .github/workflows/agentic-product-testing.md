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
  mentions: false
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
    protected-files: fallback-to-issue
    draft: false
  jobs:
    link-real-world-outputs:
      description: Cross-link the Agentic Product Testing Discussion and durable-memory PR after the built-in safe outputs run. Call this whenever a real-world result PR is requested.
      runs-on: ubuntu-latest
      needs: safe_outputs
      output: Linked the Agentic Product Testing Discussion and PR.
      permissions:
        contents: read
        discussions: write
        pull-requests: write
      inputs:
        discussion_title:
          description: Exact title passed to create_discussion, without the configured title prefix unless already included.
          required: true
          type: string
        entry_id:
          description: Real-world result entry id returned by real_world_record_result.
          required: false
          type: string
      steps:
        - name: Link PR and Discussion
          uses: actions/github-script@v9
          env:
            CREATED_PR_NUMBER: ${{ needs.safe_outputs.outputs.created_pr_number }}
            CREATED_PR_URL: ${{ needs.safe_outputs.outputs.created_pr_url }}
            WORKFLOW_RUN_URL: ${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}
          with:
            script: |
              const fs = require('fs');

              const outputPath = process.env.GH_AW_AGENT_OUTPUT;
              const prNumber = process.env.CREATED_PR_NUMBER;
              const prUrl = process.env.CREATED_PR_URL;
              const runUrl = process.env.WORKFLOW_RUN_URL;
              const owner = context.repo.owner;
              const repo = context.repo.repo;

              if (!outputPath || !fs.existsSync(outputPath)) {
                core.setFailed('GH_AW_AGENT_OUTPUT was not available to link real-world outputs.');
                return;
              }

              const agentOutput = JSON.parse(fs.readFileSync(outputPath, 'utf8'));
              const items = Array.isArray(agentOutput.items) ? agentOutput.items : [];
              const linkItem = items.find((item) => item.type === 'link_real_world_outputs');
              if (!linkItem) {
                core.setFailed('The agent did not call link_real_world_outputs.');
                return;
              }

              const discussionItem = items.find((item) => item.type === 'create_discussion');
              const pullRequestItem = items.find((item) => item.type === 'create_pull_request');
              if (!prNumber || !prUrl) {
                if (!pullRequestItem) {
                  core.warning('link_real_world_outputs was called, but no create_pull_request output was requested; skipping cross-link.');
                  return;
                }
                core.setFailed('create_pull_request was requested, but safe outputs did not expose a created PR URL/number.');
                return;
              }
              if (!discussionItem) {
                core.setFailed('link_real_world_outputs requires a matching create_discussion output.');
                return;
              }
              const requestedTitle = linkItem.discussion_title || discussionItem?.title || '';
              if (!requestedTitle) {
                core.setFailed('link_real_world_outputs requires discussion_title.');
                return;
              }
              const expectedTitle = requestedTitle.startsWith('Agentic Product Testing: ')
                ? requestedTitle
                : `Agentic Product Testing: ${requestedTitle}`;

              const discussionData = await github.graphql(
                `query($owner: String!, $repo: String!) {
                  repository(owner: $owner, name: $repo) {
                    discussions(first: 100, orderBy: {field: UPDATED_AT, direction: DESC}) {
                      nodes { id number title url body updatedAt }
                    }
                  }
                }`,
                { owner, repo },
              );
              const discussions = discussionData.repository.discussions.nodes;
              const exactTitleMatches = discussions.filter((item) => item.title === expectedTitle);
              const discussion = exactTitleMatches.find((item) => item.body && item.body.includes(runUrl))
                || exactTitleMatches[0]
                || discussions.find((item) => item.body && item.body.includes(runUrl));
              if (!discussion) {
                core.setFailed(`Could not find the Agentic Product Testing Discussion titled ${expectedTitle} or containing ${runUrl}.`);
                return;
              }

              const upsertBlock = (body, start, end, block) => {
                const text = body || '';
                const startIndex = text.indexOf(start);
                const endIndex = text.indexOf(end);
                if (startIndex !== -1 && endIndex !== -1 && endIndex > startIndex) {
                  return `${text.slice(0, startIndex)}${block}${text.slice(endIndex + end.length)}`;
                }
                return `${text.trimEnd()}\n\n${block}`;
              };

              const entryLine = linkItem.entry_id ? [`- Real-world entry: \`${linkItem.entry_id}\``] : [];
              const prBlock = [
                '<!-- real-world-output-links:start -->',
                '### Agentic Product Testing links',
                '',
                `- Discussion: ${discussion.url}`,
                ...entryLine,
                `- Workflow run: ${runUrl}`,
                '- Durable memory: this PR should be merged in order to retain these results for future MCP history queries.',
                '<!-- real-world-output-links:end -->',
              ].join('\n');

              const discussionBlock = [
                '<!-- real-world-output-links:start -->',
                '### Durable memory PR',
                '',
                `- Pull request: ${prUrl}`,
                ...entryLine,
                `- Workflow run: ${runUrl}`,
                '- This PR should be merged in order to retain these results in repo memory for future Agentic Product Testing sweeps.',
                '<!-- real-world-output-links:end -->',
              ].join('\n');

              const pr = await github.rest.pulls.get({ owner, repo, pull_number: Number(prNumber) });
              const nextPrBody = upsertBlock(pr.data.body || '', '<!-- real-world-output-links:start -->', '<!-- real-world-output-links:end -->', prBlock);
              if (nextPrBody !== (pr.data.body || '')) {
                await github.rest.pulls.update({ owner, repo, pull_number: Number(prNumber), body: nextPrBody });
              }

              const nextDiscussionBody = upsertBlock(discussion.body || '', '<!-- real-world-output-links:start -->', '<!-- real-world-output-links:end -->', discussionBlock);
              if (nextDiscussionBody !== (discussion.body || '')) {
                await github.graphql(
                  `mutation($discussionId: ID!, $body: String!) {
                    updateDiscussion(input: {discussionId: $discussionId, body: $body}) {
                      discussion { url }
                    }
                  }`,
                  { discussionId: discussion.id, body: nextDiscussionBody },
                );
              }

              core.info(`Linked ${prUrl} and ${discussion.url}.`);

timeout-minutes: 90
---

# Agentic Product Testing

Run an Agentic Product Testing sweep for DollarLint, record the structured result, and open one GitHub Discussion with the results.

Use the `dollarlint-repo` MCP server as the workflow source of truth. Start with `real_world_start_testing`, then follow the `nextStep` guidance returned by the `real_world_*` tools until the run is recorded. Prefer the MCP wizard over shell commands and hand-written reports. If the MCP capabilities are missing or stale, stop and publish a blocker summary instead of improvising.

Manual dispatch inputs for this run:
- max_repos: `${{ github.event.inputs.max_repos }}`
- candidate_repos: `${{ github.event.inputs.candidate_repos }}`

Before calling `real_world_start_testing`, derive the repository plan from those literal inputs. Treat a missing or empty `max_repos` as `10`. If `candidate_repos` is a non-empty JSON array, parse it and pass at most `max_repos` entries exactly to `real_world_start_testing.repositories`; do not substitute different repositories unless MCP history reports duplicates and `allowPreviouslyTested` is false. If `candidate_repos` is empty, choose up to `max_repos` diverse, well-known public repositories with conventional config files, skipping repositories already in MCP history unless the manual input explicitly asks for reruns.

The workflow pre-builds `bin/dollarlint`; when the MCP flow asks for validation arguments, use `build: false` so validation uses the prebuilt CLI. Keep long-running prep or validation MCP calls open for progress notifications, and do not poll with shell sleep loops. Never run dependency lifecycle scripts, postinstall hooks, package-manager plugins, or repository install scripts.

Durable repository memory must be written through `real_world_record_result` and the structured JSON run directory it manages under `reports/agentic-product-testing/<run-id>/`. Do not create Markdown report files or a shared summary index. Product recommendations are mandatory: include a `high`, `med`, or `low` strength with rationale, or record an explicit no-change recommendation when DollarLint behaved reasonably. If `real_world_record_result` changes repository files, `create_pull_request` is mandatory because merging that PR is how the repo remembers tested repositories. The PR body must say that it should be merged in order to retain the results in Agentic Product Testing memory.

After recording, create exactly one GitHub Discussion through the configured safe output in the `Agentic Product Testing` category. Keep it concise: result counts, tested repositories, notable findings, product recommendations with strength labels, persisted artifact path, DollarLint commit, and workflow run URL. Put verbose examples or raw warnings inside `<details>` blocks. The Discussion body must include a "Durable memory PR" section saying that a companion PR will be opened and that the PR should be merged in order to retain the results for future sweeps. If the Discussion falls back to an issue, say that the intended destination was a Discussion.

The workflow may expose safe outputs either as direct tools or through a `safeoutputs` CLI wrapper. Use the available safe-output interface, but do not stop after committing locally. If using the CLI wrapper, send inline JSON on stdin with `printf '%s' '<json>' | safeoutputs create_pull_request .` and `printf '%s' '<json>' | safeoutputs create_discussion .`; do not rely on temporary payload files.

After requesting both `create_pull_request` and `create_discussion`, call `link_real_world_outputs` with the exact Discussion title and the recorded entry id. This post-safe-output step cross-links the final PR and Discussion URLs and fails if no PR was created.

Do not fabricate results. If cloning, preparation, validation, triage, or recording blocks before a meaningful sweep completes, publish a short blocker summary with any partial artifacts and the next concrete fix.

Final response: choose exactly one outcome. Either recommend product changes to consider with strength and rationale grounded in the MCP record, or state that the product behaved reasonably and no product change is recommended. Do not finish with only raw counts or run mechanics.
