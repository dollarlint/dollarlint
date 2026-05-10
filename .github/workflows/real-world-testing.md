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
    title-prefix: "Real-world testing: "
    max: 1
    fallback-to-issue: true
    close-older-discussions: true
    expires: 30
  create-pull-request:
    title-prefix: "Update real-world test results: "
    max: 1
    labels: [agentic-workflows]
    if-no-changes: error
    protected-files: fallback-to-issue
    draft: false
  jobs:
    link-real-world-outputs:
      description: Cross-link the real-world testing Discussion and durable-memory PR after the built-in safe outputs run. Call this whenever a real-world result PR is requested.
      runs-on: ubuntu-latest
      needs: safe_outputs
      output: Linked the real-world testing Discussion and PR.
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

              if (!prNumber || !prUrl) {
                core.setFailed('No pull request was created. Real-world result files are durable memory; request create_pull_request whenever real_world_record_result changes repository files.');
                return;
              }
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
              const requestedTitle = linkItem.discussion_title || discussionItem?.title || '';
              if (!requestedTitle) {
                core.setFailed('link_real_world_outputs requires discussion_title.');
                return;
              }
              const expectedTitle = requestedTitle.startsWith('Real-world testing: ')
                ? requestedTitle
                : `Real-world testing: ${requestedTitle}`;

              const discussionData = await github.graphql(
                `query($owner: String!, $repo: String!) {
                  repository(owner: $owner, name: $repo) {
                    discussions(first: 25, orderBy: {field: UPDATED_AT, direction: DESC}) {
                      nodes { id number title url body }
                    }
                  }
                }`,
                { owner, repo },
              );
              const discussions = discussionData.repository.discussions.nodes;
              const discussion = discussions.find((item) => item.title === expectedTitle)
                || discussions.find((item) => item.body && item.body.includes(runUrl));
              if (!discussion) {
                core.setFailed(`Could not find the real-world testing Discussion titled ${expectedTitle}.`);
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
                '### Real-world testing links',
                '',
                `- Discussion: ${discussion.url}`,
                ...entryLine,
                `- Workflow run: ${runUrl}`,
                '- Durable memory: merge this PR to make these tested repositories visible to future MCP history queries.',
                '<!-- real-world-output-links:end -->',
              ].join('\n');

              const discussionBlock = [
                '<!-- real-world-output-links:start -->',
                '### Durable memory PR',
                '',
                `- Pull request: ${prUrl}`,
                ...entryLine,
                `- Workflow run: ${runUrl}`,
                '- Merge the PR to persist this sweep in repo memory for future real-world testing.',
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

# Real-World Testing

Run a real-world DollarLint sweep for up to `MAX_REPOS` fresh public repositories, record the structured result, and open one GitHub Discussion with the results.

Use the `dollarlint-repo` MCP server as the workflow source of truth. Start with `real_world_start_testing`, then follow the `nextStep` guidance returned by the `real_world_*` tools until the run is recorded. Prefer the MCP wizard over shell commands and hand-written reports. If the MCP capabilities are missing or stale, stop and publish a blocker summary instead of improvising.

If `CANDIDATE_REPOS` is a non-empty JSON array, use it as the candidate pool. Otherwise choose up to `MAX_REPOS` diverse, well-known public repositories with conventional config files, skipping repositories already in MCP history unless the manual input explicitly asks for reruns.

The workflow pre-builds `bin/dollarlint`; when the MCP flow asks for validation arguments, use `build: false` so validation uses the prebuilt CLI. Keep long-running prep or validation MCP calls open for progress notifications, and do not poll with shell sleep loops. Never run dependency lifecycle scripts, postinstall hooks, package-manager plugins, or repository install scripts.

Durable repository memory must be written through `real_world_record_result` and the structured JSON files it manages. Do not create Markdown report files. Product recommendations are mandatory: include a `high`, `med`, or `low` strength with rationale, or record an explicit no-change recommendation when DollarLint behaved reasonably. If `real_world_record_result` changes repository files, `create_pull_request` is mandatory because merging that PR is how the repo remembers tested repositories. The PR body must say that merging it persists real-world testing memory.

After recording, create exactly one GitHub Discussion through the configured safe output. Keep it concise: result counts, tested repositories, notable findings, product recommendations with strength labels, persisted artifact path, DollarLint commit, and workflow run URL. Put verbose examples or raw warnings inside `<details>` blocks. The Discussion body must include a "Durable memory PR" section explaining that a companion PR will be opened and must be merged for future sweeps to see this memory. If the Discussion falls back to an issue, say that the intended destination was a Discussion.

After requesting both `create_pull_request` and `create_discussion`, call `link_real_world_outputs` with the exact Discussion title and the recorded entry id. This post-safe-output step cross-links the final PR and Discussion URLs and fails if no PR was created.

Do not fabricate results. If cloning, preparation, validation, triage, or recording blocks before a meaningful sweep completes, publish a short blocker summary with any partial artifacts and the next concrete fix.

Final response: choose exactly one outcome. Either recommend product changes to consider with strength and rationale grounded in the MCP record, or state that the product behaved reasonably and no product change is recommended. Do not finish with only raw counts or run mechanics.
