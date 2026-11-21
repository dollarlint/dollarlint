# Agent Notes

- This repository has special, optimized MCP tools for working in it. When available, prefer those tools over generic tools or CLI invocation.
- For real-world validation sweeps, start with the repo MCP `real_world_start_testing` tool and follow the `nextStep` guidance returned by each real-world tool. Durable sweep memory belongs in the MCP structured result files, not Markdown reports.
- Do not manually post-process generated `gh aw` lock files. If checking whitespace, exclude generated lock files with `git diff --check -- . ':(exclude).github/workflows/*.lock.yml'`.
- Before we reach a v1, it is completely fine to break schemas, change configuration behavior, etc. Optimize for correctness and ergonomics, even if the change is breaking. Do not worry about backward compatibility.
- Before editing the descriptions of existing pull requests, be sure to read the current contents. This helps avoid data loss.
- When changing tool functionality, including configuration options, review the public website content and update if necessary.

- Dependency maintenance: consolidate overlapping updates instead of creating another repair PR. Reuse an existing PR when practical, and report blockers there instead of opening empty PRs. Required CI checks must run on the final commit; Vercel and CodeQL alone are insufficient.
- `ci_readiness` with `job: "goreleaser-check"` requires the GoReleaser Pro CLI, matching CI. It builds a snapshot and audits the generated npm wrapper; snapshot builds do not require a license key.
- Regenerate agentic workflows with gh-aw v1.3.0 and `gh aw compile --action-mode action --action-tag 8126d6cb07dea9221848de275af8a41474d839ca`. After `--dependabot`, preserve the grouping configuration in `.github/dependabot.yml`.
