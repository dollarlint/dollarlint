# Real-World Test Report

This report records DollarLint sweeps against real public repositories. Before starting a new sweep, check the repository tables below and avoid retesting the same projects unless the goal is an intentional before/after comparison.

Structured sweep history is also stored in `reports/real-world-results.json` and declared by `reports/real-world-results.schema.json`; prefer the repo MCP `real_world_history` tool for duplicate checks and queries.

## 2026-05-09 - Initial 10-Repo Corpus

- DollarLint revision: `417cc4874097d0678a0f176a6938619de55b6c23` with active working-tree product changes during the session.
- Corpus: `/private/tmp/dollarlint-corpus.SBjYew`
- Representative final command: `bin/dollarlint validate /private/tmp/dollarlint-corpus.SBjYew --schema-store --schema-store-failure warn --fetch-retries 1 --fetch-retry-min-wait 1ms --fetch-retry-max-wait 1ms --format json --output /private/tmp/dollarlint-corpus-auto.json`
- Output artifact: `/private/tmp/dollarlint-corpus-auto.json`

### Repositories

| Repo | Ecosystem | Clone URL | Repo commit | Notes |
| --- | --- | --- | --- | --- |
| cargo | Rust | https://github.com/rust-lang/cargo.git | `a343accce852` | Rustfix JSON fixtures and Cargo.toml test fixtures produced expected failures. |
| efcore | C#/.NET | https://github.com/dotnet/efcore.git | `a267c3e860df` | Devcontainer schema exposed unsupported `vscode:` refs; Azure pipeline validation had likely schema/version mismatch. |
| fmt | C++ | https://github.com/fmtlib/fmt.git | `9cb8c0f92b4c` | No notable final findings. |
| laravel-framework | PHP | https://github.com/laravel/framework.git | `295448c0076a` | No notable final findings. |
| maven | Java | https://github.com/apache/maven.git | `9cedc78f3491` | No notable final findings. |
| prometheus | Go | https://github.com/prometheus/prometheus.git | `ecab2f45a8b7` | Invalid YAML/JSON testdata and one ESLint validation issue. |
| rails | Ruby | https://github.com/rails/rails.git | `7aeda6a9c478` | Test fixtures accounted for most parse findings; devcontainer schema exposed unsupported `vscode:` refs. |
| requests | Python | https://github.com/psf/requests.git | `eb173bc819c7` | GitHub workflow SchemaStore validation reported matrix-related issues. |
| typescript | TypeScript | https://github.com/microsoft/TypeScript.git | `f350b5233149` | Large number of JSON baseline/test artifacts with multiple JSON values. |
| zig | Zig | https://github.com/ziglang/zig.git | `738d2be9d6b6` | Forgejo issue template files matched GitHub issue-form schema too eagerly. |

### Result

| Metric | Count |
| --- | ---: |
| Discovered | 4055 |
| Validated | 1073 |
| Skipped | 2269 |
| Failed | 713 |
| Issues | 735 |
| Parsing issues | 713 |
| Validation issues | 22 |
| Schema issues | 0 |
| Warnings | 4 |

### Finding Breakdown

| Repo | Total | Parsing | Validation | Schema | Notes |
| --- | ---: | ---: | ---: | ---: | --- |
| cargo | 24 | 13 | 11 | 0 | Rustfix recorded JSON streams and intentionally invalid Cargo.toml fixtures. |
| efcore | 2 | 0 | 2 | 0 | Azure pipeline values likely accepted by Azure tooling but not this schema branch. |
| prometheus | 6 | 5 | 1 | 0 | Deliberately bad YAML, `_ping.json` files containing `OK`, large JSON stream/testdata. |
| rails | 11 | 10 | 1 | 0 | Mostly test fixtures/baselines; workflow schema issue may be SchemaStore strictness. |
| requests | 3 | 0 | 3 | 0 | GitHub Actions workflow schema complaints around `matrix`. |
| typescript | 685 | 685 | 0 | 0 | Baseline/test artifacts ending in `.json` but containing non-document/stream-like data. |
| zig | 4 | 0 | 4 | 0 | Forgejo issue templates matched GitHub issue-form expectations. |

### Findings

- Most parse failures were real from DollarLint's perspective but not product bugs: intentionally invalid fixtures, recorded compiler output, JSON streams, placeholder responses, and test baselines that are not standalone JSON documents.
- Strict `.json` parsing was too rigid for the ecosystem expectation that many tools accept comments and trailing commas.
- JSON output needed a cleaner pre-v1 contract so scripts can distinguish parsing, validation, schema, ignored, and warning outcomes without inference.
- SchemaStore matching was too eager for generic basenames and some cross-platform conventions.
- Devcontainer schemas reference `vscode:` URIs, which caused catalog schema warnings and skipped validation.

### Product Decisions

- Default `.json` parsing should be lenient-by-default through `parsing.json.mode = "auto"`: strict JSON first, JSONC fallback for comments/trailing commas.
- Add parse hints for common real-world failure modes such as multiple JSON values, invalid test fixtures, duplicate YAML keys, templated YAML, and empty/placeholder files.
- Break and clean up JSON output before v1: `formatVersion`, relative `path`, root-relative local schema paths, `issues` and `ignoredIssues`, per-issue `category`, structured warnings, always-present arrays, and numeric `summary.durationNanos`.
- Add `[schemas.catalogs] match = "auto" # auto | all` so the default catalog behavior can avoid low-confidence matches while still offering exhaustive matching when requested.
- Treat unsupported external editor schema references like `vscode:` as a product compatibility issue rather than a user-facing warning.

## 2026-05-09 - Second 10-Repo Corpus And Priority Fixes

- DollarLint revision: `417cc4874097d0678a0f176a6938619de55b6c23` with active working-tree product changes during the session.
- Corpus: `/private/tmp/dollarlint-corpus2.oDzlq4`
- Baseline output artifact before priority fixes: `/private/tmp/dollarlint-corpus2.json`
- Final command: `bin/dollarlint validate /private/tmp/dollarlint-corpus2.oDzlq4 --schema-store --schema-store-failure warn --fetch-retries 1 --fetch-retry-min-wait 1ms --fetch-retry-max-wait 1ms --format json --output /private/tmp/dollarlint-corpus2-priority.json`
- Final output artifact: `/private/tmp/dollarlint-corpus2-priority.json`

### Repositories

| Repo | Ecosystem | Clone URL | Repo commit | Notes |
| --- | --- | --- | --- | --- |
| avalonia | C#/.NET | https://github.com/AvaloniaUI/Avalonia.git | `dcb607179b70` | Exposed UTF-8 BOM files and Azure Pipelines schema strictness. |
| cargo | Rust | https://github.com/rust-lang/cargo.git | `a343accce852` | Same Rustfix/Cargo fixture patterns as initial corpus. |
| flask | Python | https://github.com/pallets/flask.git | `7374c85ddefc` | No notable final findings. |
| flutter-samples | Dart/Flutter/Angular | https://github.com/flutter/samples.git | `56bf76f2f091` | Exposed bogus `*.app.json` SchemaStore match for `tsconfig.app.json`; Angular schema pointed into missing `node_modules`. |
| julia | Julia | https://github.com/JuliaLang/julia.git | `2569364ac493` | No notable final findings. |
| laravel-framework | PHP | https://github.com/laravel/framework.git | `295448c0076a` | No notable final findings. |
| phoenix | Elixir | https://github.com/phoenixframework/phoenix.git | `e50429581546` | No notable final findings. |
| prometheus | Go | https://github.com/prometheus/prometheus.git | `ecab2f45a8b7` | Same invalid testdata pattern as initial corpus. |
| svelte | TypeScript/Svelte | https://github.com/sveltejs/svelte.git | `af5b9724ab31` | No notable final findings. |
| zig | Zig | https://github.com/ziglang/zig.git | `738d2be9d6b6` | Same Forgejo issue-template SchemaStore mismatch pattern. |

### Before Priority Fixes

| Metric | Count |
| --- | ---: |
| Discovered | 2079 |
| Validated | 894 |
| Skipped | 1164 |
| Failed | 21 |
| Issues | 54 |
| Parsing issues | 21 |
| Validation issues | 32 |
| Schema issues | 1 |
| Warnings | 2 |

### After Priority Fixes

| Metric | Count |
| --- | ---: |
| Discovered | 2079 |
| Validated | 898 |
| Skipped | 1163 |
| Failed | 18 |
| Issues | 45 |
| Parsing issues | 18 |
| Validation issues | 26 |
| Schema issues | 1 |
| Warnings | 0 |

### Final Finding Breakdown

| Repo | Total | Parsing | Validation | Schema | Notes |
| --- | ---: | ---: | ---: | ---: | --- |
| avalonia | 9 | 0 | 9 | 0 | Azure Pipelines branch/schema strictness and one ESLint config issue. |
| cargo | 24 | 13 | 11 | 0 | Recorded JSON streams and intentionally invalid Cargo.toml fixtures. |
| flutter-samples | 2 | 0 | 1 | 1 | GitHub workflow `matrix` validation; Angular local schema missing from `node_modules`. |
| prometheus | 6 | 5 | 1 | 0 | Deliberately bad YAML, placeholder `_ping.json`, and JSON stream/testdata. |
| zig | 4 | 0 | 4 | 0 | Forgejo issue templates matched GitHub issue-form expectations. |

### Findings

- Avalonia had UTF-8 BOM-prefixed configuration files; those should parse normally.
- SchemaStore's `*.app.json` match incorrectly captured Flutter's `tsconfig.app.json` before the more appropriate TypeScript schema.
- Devcontainer `vscode:` references produced catalog schema warnings.
- Stale persistent cache entries for reused localhost test-server URLs made plain `go test ./...` unreliable during local development.
- Remaining parse findings still look real or fixture-oriented: JSON streams/multiple JSON values, deliberately invalid TOML/YAML fixtures, and placeholder files with non-JSON contents.

### Product Decisions

- Strip UTF-8 BOMs before parsing documents, decoding schemas, and attaching source maps.
- In `match = "auto"`, skip basename-only leading wildcard SchemaStore globs such as `*.app.json`; preserve them under `match = "all"`.
- Treat `vscode:` schema references as permissive empty schemas so catalog-backed validation can continue without warnings.
- Do not persist remote schema cache entries for loopback/local hosts; in-memory per-run caching is enough for local schema servers and avoids stale test pollution.

### Follow-up

- Final corpus result improved from 54 to 45 issues.
- Parsing issues dropped from 21 to 18.
- Validation issues dropped from 32 to 26.
- Warnings dropped from 2 to 0.
- Validated file count rose from 894 to 898.

## 2026-05-09 - Fresh 6-Repo Cross-Ecosystem Corpus

- DollarLint revision: `c63b83b482d9c8ef8148c6f151742571fc20ae8f` with a clean working tree.
- Corpus: `/tmp/dollarlint-corpus.CtwOPD`
- Command: `XDG_CACHE_HOME=/tmp/dollarlint-cache.V2T8k4 bin/dollarlint validate /tmp/dollarlint-corpus.CtwOPD --schema-store --schema-store-failure warn --fetch-retries 1 --fetch-retry-min-wait 1ms --fetch-retry-max-wait 1ms --format json --output /tmp/dollarlint-corpus-third.json`
- Output artifact: `/tmp/dollarlint-corpus-third.json`

### Repositories

| Repo | Ecosystem | Clone URL | Repo commit | Notes |
| --- | --- | --- | --- | --- |
| express | JavaScript/Node | https://github.com/expressjs/express.git | `f873ac23124ffcff8c040b4bd257b32c29828d53` | All discovered configuration files validated. |
| django | Python/JavaScript | https://github.com/django/django.git | `4d455ae2d7689ce066dfffef9fc29a6f6d3ed33e` | Private `package.json` uses uppercase `name`, which package-name schema validation rejects. |
| terraform | Go/HCL | https://github.com/hashicorp/terraform.git | `527402d3fe2de2363c4587e7abd1a3b23669ca25` | Invalid fixture files produced expected parse failures; Changie config exposed schema drift/strictness. |
| tokio | Rust | https://github.com/tokio-rs/tokio.git | `ee0dc9092665a1f13df573dc5e5124999d8e9035` | All discovered configuration files validated or skipped cleanly. |
| vue-core | TypeScript/Vue | https://github.com/vuejs/core.git | `57545e958ae28ed17aa9e0ed321abcd8dc99f752` | Vercel catalog schema failed metaschema validation and was skipped with a warning. |
| homebrew-brew | Ruby | https://github.com/Homebrew/brew.git | `e5cb8682f9beba3b712ee81d303dd496904ea848` | All discovered configuration files validated or skipped cleanly. |

### Result

| Metric | Count |
| --- | ---: |
| Discovered | 681 |
| Validated | 139 |
| Skipped | 540 |
| Failed | 2 |
| Issues | 4 |
| Parsing issues | 2 |
| Validation issues | 2 |
| Schema issues | 0 |
| Coverage issues | 0 |
| Ignored issues | 0 |
| Warnings | 1 |

### Finding Breakdown

| Repo | Total | Parsing | Validation | Schema | Notes |
| --- | ---: | ---: | ---: | ---: | --- |
| django | 1 | 0 | 1 | 0 | `package.json` has `"name": "Django"`; SchemaStore package schema enforces lowercase package names even for private packages. |
| terraform | 3 | 2 | 1 | 0 | `internal/configs/testdata/invalid-files/*.tf.json` files are deliberately invalid; `.changie.yaml` lacks schema-required `headerPath` while using `versionFooterPath`. |
| vue-core | 0 | 0 | 0 | 0 | `packages-private/sfc-playground/vercel.json` validation was skipped after the catalog Vercel schema failed metaschema validation. |

### Findings

- The two Terraform parsing failures are intentional invalid test fixtures: one contains native Terraform syntax in a `*.tf.json` file and the other is zero bytes.
- Django's private `package.json` is valid JSON but violates the package-name pattern in the SchemaStore package schema because the name is uppercase.
- Terraform's `.changie.yaml` appears to use a newer or project-specific Changie footer field shape than the fetched `https://changie.dev/schema.json` accepts.
- SchemaStore's Vercel schema at `https://openapi.vercel.sh/vercel.json` failed draft-04 metaschema validation because `exclusiveMinimum` is numeric where draft-04 expects a boolean; DollarLint warned and skipped catalog-inferred validation instead of crashing.
- No crashes, hangs, output-contract problems, or unexplained skipped files appeared in this sweep.

### Product Decisions

- Improve catalog schema failure warnings so users can tell the inferred schema failed, not their file.
- Keep reporting intentionally invalid fixture files as parse failures; this is expected behavior for full-repository sweeps.
- Treat the Vercel schema warning as a third-party schema compatibility signal for now, not a local suppression candidate.
- Keep package and Changie validation findings visible; they appear to be schema/project mismatches rather than DollarLint parser bugs.

### Follow-up

- Implemented the warning-copy follow-up in `internal/engine/validation_compile.go`: warn-mode catalog schema failures now lead with a plain-language message that the inferred schema could not be used and this is not a finding in the file, while preserving compiler details in `warning.hint`.
- Verified the follow-up with `go test ./internal/engine -run TestSchemaStoreMatchedSchemaFailurePolicy -count=1`, repo quick verification (`go test ./...` and `go vet ./...`), and a rerun against `/tmp/dollarlint-corpus.CtwOPD` to inspect the JSON warning shape.
- If Vercel schema warnings recur across additional real projects, consider testing whether permissive schema compilation or a targeted upstream report would improve catalog-backed validation without hiding broken schemas.

## 2026-05-09 - Fresh 5-Repo Config Sweep

- DollarLint revision: `2dc177328588108815b51f713032270315e8419d` with unrelated untracked setup/editor files present (`.gitattributes`, `.github/agents/`, `.github/workflows/copilot-setup-steps.yml`, `.vscode/`).
- Corpus: `/var/folders/dg/0y8q_bz169jbjnv14dwmw0dc0000gn/T/dollarlint-corpus.4217735361`
- Dependency prep: none; this sweep used raw shallow clones and did not install target-repo dependencies before validation.
- Command: `XDG_CACHE_HOME="/var/folders/dg/0y8q_bz169jbjnv14dwmw0dc0000gn/T/dollarlint-cache.3334137466" bin/dollarlint validate "/var/folders/dg/0y8q_bz169jbjnv14dwmw0dc0000gn/T/dollarlint-corpus.4217735361" --schema-store --schema-store-failure warn --fetch-retries 1 --fetch-retry-min-wait 1ms --fetch-retry-max-wait 1ms --format json --output "/var/folders/dg/0y8q_bz169jbjnv14dwmw0dc0000gn/T/dollarlint-fresh-5-repo-config-sweep-1156465496.json"`
- Output artifact: `/var/folders/dg/0y8q_bz169jbjnv14dwmw0dc0000gn/T/dollarlint-fresh-5-repo-config-sweep-1156465496.json`

### Repositories

| Repo | Ecosystem | Clone URL | Repo commit | Notes |
| --- | --- | --- | --- | --- |
| vite | TypeScript/Vite | https://github.com/vitejs/vite.git | `cf0ff4154b26cffbf18541ade1a50818842731d3` | Empty/malformed test fixtures plus package/pnpm workspace fixture schema mismatches. |
| react | JavaScript/React | https://github.com/facebook/react.git | `d5736f098edee62c44f27b053e6e48f5fa443803` | No file issues; catalog schema warnings for Vercel and CircleCI inferred schemas. |
| ansible | Python/YAML | https://github.com/ansible/ansible.git | `b7c0900272fd428f336f30714089e3916fcc10f9` | High-volume Ansible task/playbook/meta schema noise in integration fixtures. |
| grafana | Go/TypeScript | https://github.com/grafana/grafana.git | `a0cc3f51fb9be55db3d3742a85c9411f067bd201` | Broken testdata, Docker Compose fragments, plugin/OpenAPI fixtures, and missing local Nx/Lerna schemas. |
| deno | Rust/TypeScript | https://github.com/denoland/deno.git | `d6212d40304e3d50f3337bfff0a627f11916b5f0` | Empty/malformed fixture JSON/YAML and intentionally unusual tsconfig/package/jsr fixtures. |

### Result

| Metric | Count |
| --- | ---: |
| Discovered | 8106 |
| Validated | 3184 |
| Skipped | 4854 |
| Failed | 68 |
| Issues | 935 |
| Parsing issues | 68 |
| Validation issues | 836 |
| Schema issues | 31 |
| Coverage issues | 0 |
| Ignored issues | 0 |
| Warnings | 2 |

### Finding Breakdown

| Repo | Total | Parsing | Validation | Schema | Notes |
| --- | ---: | ---: | ---: | ---: | --- |
| vite | 6 | 2 | 4 | 0 | Empty/malformed fixture JSON plus package and pnpm workspace fixture validation mismatches. |
| react | 0 | 0 | 0 | 0 | No file issues; two catalog schema-unavailable warnings. |
| ansible | 711 | 32 | 679 | 0 | Mostly Ansible task/playbook/meta schema validation noise in integration fixtures; parse findings are deliberately invalid JSON/YAML fixtures. |
| grafana | 175 | 8 | 136 | 31 | Empty/broken testdata, Docker Compose fragments, plugin/OpenAPI fixture mismatches, and missing local `node_modules` schemas. |
| deno | 43 | 26 | 17 | 0 | Empty/malformed test fixture JSON/YAML, shebang JSON fixtures, package-name casing fixtures, and tsconfig enum mismatches. |

### Findings

- React produced no file issues; only catalog schema-unavailable warnings for the known Vercel draft-04 incompatibility and a CircleCI schema metaschema problem.
- Ansible produced a large number of validation findings because SchemaStore's Ansible task/playbook/meta schemas are stricter than many integration fixture patterns, especially boolean `yes` values and test-only task shapes.
- Grafana exposed two distinct expected-noise classes: missing local `node_modules` schemas from `$schema` references because dependencies were not installed, and partial Docker Compose block files that are not standalone compose documents.
- Deno and Vite mostly reinforced existing fixture behavior: empty files, deliberately malformed configs, shebang JSON, and edge-case package/tsconfig fixtures should remain visible in full-repository sweeps.
- No crashes, hangs, unexplained warnings, output-contract regressions, or unclassified skipped-file problems appeared in this sweep.

### Product Decisions

- Keep reporting intentionally malformed fixture files and partial config fragments in full-repository sweeps; users can exclude those paths when they want application-only validation.
- Consider improving schema issue hints for missing local `file://` schemas so users can tell that dependencies such as `node_modules` were not installed before validation.
- Treat the high-volume Ansible task-schema findings as schema/project mismatch and reporting-noise pressure, not a parser fix.
- Keep watching recurring catalog schema compile failures. The Vercel warning is already tracked; CircleCI's duplicate enum metaschema failure is a new recurrence candidate if it appears in more repos.

### Follow-up

- Structured result recorded in `reports/real-world-results.json`.
- Updated the real-world testing skill after this sweep to require context-sensitive dependency prep before validation when dependencies affect schema/config fidelity.
- No product code changes were made during this sweep.
