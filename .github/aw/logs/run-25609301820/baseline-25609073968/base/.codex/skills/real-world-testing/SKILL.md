---
name: real-world-testing
description: Real-world validation workflow for DollarLint. Use when Codex is asked to test DollarLint against public GitHub repositories, run corpus/regression sweeps, evaluate beta rough edges on real projects, compare product behavior across ecosystems, or update reports/REAL-WORLD-TESTS.md with tested repos, findings, and product decisions.
---

# Real-World Testing

## Workflow

1. Query real-world history first.
   - When the repo-specific MCP tools are available, start with `real_world_history` to inspect the schema-declared `reports/real-world-results.json` history and check whether candidate repos have already been tested.
   - Also skim `reports/REAL-WORLD-TESTS.md` for narrative context and product decisions that may not fit neatly in structured data.
   - Do not retest repos already listed in either source unless the user asks for a rerun, you need a before/after comparison, or a prior entry says the repo should be revisited.
   - Note prior product decisions so you do not rediscover the same conclusion as if it were new.

2. Record the DollarLint revision under test.
   - Capture `git rev-parse HEAD`.
   - If the working tree is dirty, say so in the report and describe the relevant uncommitted scope.
   - Use the repo-specific MCP verification tools when available, but keep raw commands/results reproducible in the report.

3. Choose real repositories deliberately.
   - Prefer public, well-known projects with different ecosystems and config styles.
   - Include each repo's clone URL, checked-out commit SHA, and ecosystem/language in the report.
   - Prefer `real_world_prepare_corpus` to create `/tmp/dollarlint-corpus.<id>`, an isolated cache directory, an output artifact path, and a `real-world-manifest.json` file.
   - Clone into the prepared temp directory, not into the repository. Let `real_world_prepare_corpus` perform shallow clones when useful, or use its returned clone commands.
   - Avoid repeating repos from earlier report entries unless retesting is intentional.

4. Prepare each cloned repository's dependency context when it affects validation.
   - Before running DollarLint, inspect each clone for dependency metadata that may provide local schemas or config context, especially `$schema` references into `node_modules`, package-manager lockfiles, or tool-specific schema packages.
   - Install dependencies when doing so is realistic, bounded, and likely to improve validation fidelity. Prefer reproducible, lockfile-backed, script-suppressed installs:
     - `npm ci --ignore-scripts` when `package-lock.json` is present.
     - `pnpm install --frozen-lockfile --ignore-scripts` when `pnpm-lock.yaml` is present and pnpm/corepack is available.
     - `yarn install --immutable --ignore-scripts` for modern Yarn lockfiles when Yarn is available; use the repo's documented install command when it is safer or clearer.
     - Ecosystem equivalents such as `go mod download`, `bundle install`, or Python dependency sync only when they are needed for schema/config discovery and can be run without starting services or building the project.
   - Keep installs isolated to the temporary corpus clones. Do not install dependencies in the DollarLint repository itself unless the sweep explicitly targets this repo.
   - Avoid long-running, externally visible, or service-starting setup such as Docker Compose, database provisioning, postinstall side effects, or full build/test commands unless the user explicitly asks for that depth.
   - If an install is skipped, fails, times out, or is intentionally narrowed, continue the sweep but record the command, failure/skip reason, and expected effect on validation. Treat missing local `file://` or `node_modules` schemas differently when dependencies were not installed.

5. Run DollarLint in a realistic but bounded mode.
   - Build the CLI from the revision under test: `go build -o bin/dollarlint ./cmd/dollarlint`.
   - Prefer `real_world_run_corpus` for the standard real-world command and artifact capture.
   - Prefer an isolated cache for reproducibility:

```bash
cache="$(mktemp -d /tmp/dollarlint-cache.XXXXXX)"
XDG_CACHE_HOME="$cache" bin/dollarlint validate "$corpus" \
  --schema-store \
  --schema-store-failure warn \
  --fetch-retries 1 \
  --fetch-retry-min-wait 1ms \
  --fetch-retry-max-wait 1ms \
  --format json \
  --output "$output_json"
```

   - Record any intentional deviations, for example `--no-schema-cache`, no SchemaStore, or a narrower include set.
   - Nonzero exit code is expected when real issues are found; treat crashes, hangs, and unexplained warnings as higher-severity product signals.

6. Triage findings before changing product behavior.
   - Separate parsing, validation, schema, coverage, warning, crash/performance, and output-contract findings.
   - Account for dependency prep when interpreting schema issues. A missing local schema from an uninstalled dependency is a setup limitation or UX hint opportunity, not evidence that the target repo's config is invalid.
   - Decide whether each class is a real third-party issue, expected test fixture data, a SchemaStore mismatch, a parser compatibility gap, a confusing output/reporting problem, or a DollarLint bug.
   - Prefer product fixes when real projects use syntax or schema patterns accepted by their own tools.
   - Prefer hints, docs, or config guidance when failures are intentional fixtures or non-document test baselines.
   - You MUST summarize product recommendations before finishing the sweep. Each recommendation MUST include a strength label of `high`, `med`, or `low`, based on frequency, severity, reproducibility, and expected user impact.

7. Persist structured and narrative results.
   - Prefer `real_world_record_result` to append the structured entry to `reports/real-world-results.json`; it preserves the file's `$schema` declaration and can read summary counts from the DollarLint JSON output artifact and repositories from the prepared corpus manifest.
   - Then append the narrative entry to `reports/REAL-WORLD-TESTS.md`.
   - Add a new dated entry; do not overwrite prior entries.
   - Include: date, DollarLint commit, working-tree note, dependency-prep commands/results, validation command, output artifact path, repos tested, summary counts, findings, product recommendations with strength labels, and product changes decided.
   - When using `real_world_record_result`, put the recommendation summary in `productDecisions`; preserve each strength label in the text, for example `high: Improve missing local schema hints because ...`.
   - When product changes are made after the sweep, append the follow-up result and the decision that caused it.

## Repo MCP Tools

- `real_world_history`: query structured history and check candidate repo names or clone URLs before testing.
- `real_world_prepare_corpus`: create temp corpus/cache/output paths, flag previously tested repos, optionally shallow-clone public repos, and write a corpus manifest.
- `real_world_run_corpus`: build the CLI and run the standard SchemaStore-backed JSON validation command with isolated cache settings.
- `real_world_record_result`: save the sweep to `reports/real-world-results.json` from the manifest and output artifact.

## Report Shape

Use this structure for each entry:

```markdown
## YYYY-MM-DD - Short Title

- DollarLint revision: `<commit>` plus any working-tree note
- Corpus: `<temp dir or durable location>`
- Dependency prep: `<commands run, skipped installs, failures, or none>`
- Command: `<reproducible command>`
- Output artifact: `<json path>`

### Repositories
| Repo | Ecosystem | Clone URL | Repo commit | Notes |

### Result
| Metric | Count |

### Findings
- ...

### Product Recommendations
| Strength | Recommendation | Rationale |
| --- | --- | --- |
| high/med/low | ... | ... |

### Follow-up
- ...
```

Keep the report factual. It is a memory system for future agents, not a polished blog post.
