# Agent Notes

- This repository has special, optimized MCP tools for working in it. When available, prefer those tools over generic tools or CLI invocation.
- For real-world validation sweeps, start with the repo MCP `real_world_start_testing` tool and follow the `nextStep` guidance returned by each real-world tool. Durable sweep memory belongs in the MCP structured result files, not Markdown reports.
- Do not manually post-process generated `gh aw` lock files. If checking whitespace, exclude generated lock files with `git diff --check -- . ':(exclude).github/workflows/*.lock.yml'`.
- Before we reach a v1, it is completely fine to break schemas, change configuration behavior, etc. Optimize for correctness and ergonomics, even if the change is breaking. Do not worry about backward compatibility.
- Before editing the descriptions of existing pull requests, be sure to read the current contents. This helps avoid data loss.
