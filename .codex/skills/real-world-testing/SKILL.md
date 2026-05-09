---
name: real-world-testing
description: Real-world validation workflow for DollarLint. Use when Codex is asked to test DollarLint against public GitHub repositories, run corpus/regression sweeps, evaluate beta rough edges on real projects, compare product behavior across ecosystems, or update reports/REAL-WORLD-TESTS.md with tested repos, findings, and product decisions.
---

# Real-World Testing

## Workflow

1. Query real-world history first.
   - When the repo-specific MCP tools are available, start with `real_world_history` to inspect `reports/real-world-results.json` and check whether candidate repos have already been tested.
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

4. Run DollarLint in a realistic but bounded mode.
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

5. Triage findings before changing product behavior.
   - Separate parsing, validation, schema, coverage, warning, crash/performance, and output-contract findings.
   - Decide whether each class is a real third-party issue, expected test fixture data, a SchemaStore mismatch, a parser compatibility gap, a confusing output/reporting problem, or a DollarLint bug.
   - Prefer product fixes when real projects use syntax or schema patterns accepted by their own tools.
   - Prefer hints, docs, or config guidance when failures are intentional fixtures or non-document test baselines.

6. Persist structured and narrative results.
   - Prefer `real_world_record_result` to append the structured entry to `reports/real-world-results.json`; it can read summary counts from the DollarLint JSON output artifact and repositories from the prepared corpus manifest.
   - Then append the narrative entry to `reports/REAL-WORLD-TESTS.md`.
   - Add a new dated entry; do not overwrite prior entries.
   - Include: date, DollarLint commit, working-tree note, command, output artifact path, repos tested, summary counts, findings, and product changes decided.
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
- Command: `<reproducible command>`
- Output artifact: `<json path>`

### Repositories
| Repo | Ecosystem | Clone URL | Repo commit | Notes |

### Result
| Metric | Count |

### Findings
- ...

### Product Decisions
- ...

### Follow-up
- ...
```

Keep the report factual. It is a memory system for future agents, not a polished blog post.
