# Real-World Test Report

This report records DollarLint sweeps against real public repositories. Before starting a new sweep, check the repository tables below and avoid retesting the same projects unless the goal is an intentional before/after comparison.

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
