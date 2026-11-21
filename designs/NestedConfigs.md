# Nested Configuration

## Problem

DollarLint currently loads one `.dollarlint.toml` for a run. Directory
validation searches the target root, explicit file validation searches beside
the file, and nested `.dollarlint.toml` files are validated as ordinary TOML
files rather than applied to their subtrees.

That behavior is easy to explain, but it becomes awkward in monorepos and
repositories with independently owned directories. A root config often wants to
define shared defaults, fetch policy, default exclusions, and catalog behavior,
while a package or infrastructure subtree wants local schema associations,
local ignore rules, stricter coverage requirements, or different parsing
policy.

Users can work around this by running DollarLint separately in each subtree or
by writing large root-relative glob rules in one root config. Both options make
the tool feel less natural for repositories where ownership is already
directory-scoped.

## Goals

- Let subtrees carry local DollarLint policy in their own `.dollarlint.toml`.
- Preserve the current single-config behavior for existing projects.
- Make inheritance explicit so users can tell whether a child config includes
  root defaults.
- Keep config path and glob behavior predictable when configs live below the
  validation root.
- Avoid surprising cross-subtree effects from unrelated nested configs.
- Keep validators mostly independent of config-discovery mechanics.
- Make CLI overrides apply consistently across the whole invocation.

## Non-goals

- Do not recreate legacy ESLint-style implicit cascading through every ancestor
  config by default.
- Do not require every nested config to inherit from the root config.
- Do not make output options vary per subtree in a single run.
- Do not add multiple config file formats.
- Do not change in-file `$schema` precedence.

## Design influences

Other tools point to two useful patterns:

- TypeScript uses explicit `extends` for inheritance. A config opts into its
  base config and then overrides it locally.
- Modern ESLint flat config moved away from legacy implicit `.eslintrc`
  cascading. Config is selected for a target file by resolution rules, and an
  explicit CLI config changes that search behavior.

DollarLint should borrow the clarity of TypeScript inheritance and the
per-file resolution model of modern lint tools, while avoiding a hidden merge of
every `.dollarlint.toml` on the path.

## Proposed config

Add explicit inheritance:

```toml
extends = "../.dollarlint.toml"

[[schemas.associations]]
file = "*.yaml"
schema = "./schemas/package-settings.schema.json"
```

Add an opt-in nested config mode at the root of a run:

```toml
[configs]
mode = "nearest"
```

Suggested modes:

- `"single"`: current behavior. Load one config for the whole run.
- `"nearest"`: discover nested `.dollarlint.toml` files and apply the nearest
  effective config to each discovered file.

For version 1 configs, the default should remain `"single"`. A future version 2
config could consider making `"nearest"` the default after the behavior has
been available behind an explicit mode.

## Semantics

When `configs.mode = "single"`:

- DollarLint behaves as it does today.
- The root or explicit config is loaded once.
- Nested `.dollarlint.toml` files are discovered and validated as files when
  they match discovery rules, but they do not affect other files.

When `configs.mode = "nearest"`:

- DollarLint walks the target root and records any discovered
  `.dollarlint.toml` files.
- Each file is validated with the nearest config at or above the file's
  directory.
- A nested config is effective for its directory and descendants only.
- A nested config does not inherit from its parent unless it declares
  `extends`.
- If no config applies to a file, DollarLint uses `DefaultConfig()`.
- Only the run root or explicit config controls the run's config mode. A nested
  config's `configs.mode` matters when that nested directory is validated as
  its own run, not when it is loaded as part of a parent run.

Example:

```text
repo/
  .dollarlint.toml
  package.json
  packages/api/.dollarlint.toml
  packages/api/openapi.yaml
  packages/web/settings.json
```

With `configs.mode = "nearest"` in `repo/.dollarlint.toml`:

- `package.json` uses `repo/.dollarlint.toml`.
- `packages/api/openapi.yaml` uses `packages/api/.dollarlint.toml`.
- `packages/web/settings.json` uses `repo/.dollarlint.toml`.

If `packages/api/.dollarlint.toml` wants root defaults, it should say:

```toml
extends = "../../.dollarlint.toml"
```

## Path resolution

Nested configs need a stronger path rule than the current single-config model.
All paths authored inside a config should resolve relative to that config file's
directory:

- `extends`
- `discovery.include`
- `discovery.exclude`
- `schemas.associations[].file`
- relative `schemas.associations[].schema` values
- local `schemas.catalogs.sources[].path`
- ignore rule `file` globs

This means a subtree config can be moved with its local schemas:

```toml
[[schemas.associations]]
file = "*.toml"
schema = "./schemas/settings.schema.json"
```

For a file under `packages/api/`, the association glob is matched relative to
`packages/api/`, and the schema is resolved relative to
`packages/api/.dollarlint.toml`.

In-file `$schema` values should continue to resolve relative to the document
that declares them.

## Discovery rules

Config discovery and file discovery should be related, but not identical.

Rules:

- Config discovery looks for files named `.dollarlint.toml` regardless of
  `discovery.include`.
- Directory traversal respects the active config's excludes before descending
  into children.
- If the active config excludes a directory, DollarLint does not descend to find
  a nested config inside that directory.
- When a directory contains `.dollarlint.toml`, nearest mode loads it before
  walking that directory's children.
- Regular files are included only when they match the active config's
  `discovery.include` and do not match the active config's excludes.
- Nested `.dollarlint.toml` files are validated as ordinary documents only if
  they match the active include/exclude rules. Loading them as config is
  independent of whether they are included as validation targets.

This makes traversal work like a stack of directory-scoped policies. Root
excludes can still establish repository-wide boundaries, while a child config
can change which files are included below its own directory.

## Merge rules

Inheritance should be deterministic and documented. Load bases first, then
overlay the child.

Suggested merge behavior:

- Scalars replace parent values when the child sets them.
- Nested objects merge field by field.
- `discovery.include` replaces the parent include list.
- `discovery.exclude` appends parent entries first, then child entries.
- `schemas.associations` appends parent entries first, then child entries, but
  matching should prefer later entries so child associations can override
  parent associations.
- `ignore` appends parent entries first, then child entries, but child rules
  should be evaluated first when selecting an ignore reason.
- `schemas.catalogs.sources` merges by `name` when present, otherwise appends.
  A child source with the same `name` can change URL, path, enabled state, or
  failure behavior.
- `schemas.fetch.allowedDomains` and `schemas.fetch.blockedDomains` append.
- `output` can merge normally when a config is loaded as the root of a run.
  During a nearest-mode parent run, output should be treated as
  invocation-level behavior from the run root or explicit config plus CLI
  flags, not as per-subtree behavior.

The append lists should keep enough source metadata to report where a rule came
from in future verbose diagnostics.

## CLI behavior

CLI flags should be a final overlay across the entire invocation.

Examples:

- `--include` replaces discovery includes for all effective configs.
- `--exclude` appends to all effective configs.
- `--schema glob=uri` appends a global association to all effective configs.
- `--catalogs`, `--catalog-source`, and `--catalog-failure` apply to
  all effective configs.
- `--max-depth`, fetch flags, compile timeout, and domain policy apply to all
  effective configs.
- Output flags continue to affect the final report, not individual subtrees.

`--config path/to/.dollarlint.toml` should keep the current explicit-config
meaning: use that config for the whole run and do not discover nested configs
unless a separate future flag explicitly asks for that combination.

## Precedence

Schema selection precedence inside an effective config should remain:

1. In-file schema declaration.
2. User schema association.
3. DollarLint's built-in `.dollarlint.toml` association.
4. Catalog match.

Nested config resolution chooses which effective config is used before that
schema-selection order runs. It should not make parent associations beat a
child file's in-file `$schema`.

## Implementation shape

Keep the existing `Config` type as the public policy object, but add a resolver
layer around it.

Possible internal types:

```go
type ConfigFile struct {
	Path      string
	Dir       string
	Effective Config
}

type FileConfig struct {
	File             DiscoveredFile
	Config           Config
	ConfigPath       string
	ConfigDir        string
	RelativeToConfig string
}
```

High-level flow:

1. Resolve the run root and load the root or explicit config as today.
2. Decide config mode after root config and CLI flags are known.
3. In single mode, use the current `DiscoverFiles` behavior.
4. In nearest mode, walk from the run root while carrying an active config
   scope:
   - load a directory's `.dollarlint.toml` before walking its children,
   - resolve `extends` chains with cycle detection and a maximum depth,
   - build the directory's effective config with config-relative path metadata,
   - apply that effective config's excludes to child directories and files,
   - include regular files according to that effective config's includes.
5. Parse and validate files using their assigned config.
6. Format a single combined result.

Validation workers can still operate mostly as they do today. The main change
is that parsing, schema association, catalog matching, ignore application, and
schema-cache policy need access to the file's effective config rather than a
single global config.

Catalog loading may need to be grouped by effective catalog config so a run
does not load the same catalog repeatedly. Schema compilation cache keys should
continue to include resolved schema URIs; if fetch or compile policy varies by
subtree, the cache owner should be keyed by the effective fetch/compile policy
or kept per effective config.

## Config schema/API changes

- Add `Extends string` to `Config`.
- Add `ConfigDiscoveryConfig` or similar with `Mode string`.
- Add public constants for config modes:
  - `ConfigModeSingle`
  - `ConfigModeNearest`
- Add a config resolver internal package or engine helper.
- Add validation in `validateConfigValues`:
  - supported `configs.mode`,
  - `extends` must name `.dollarlint.toml`,
  - no absolute-path restriction unless the product wants one,
  - reject cycles when resolving an extends graph.
- Update `schemas/dollarlint.schema.json` and the embedded config schema.
- Expose new public types through top-level type aliases in `types.go`.

## Migration plan

Phase 1: support `extends` without changing discovery semantics.

This immediately helps subtree-specific manual runs:

```sh
dollarlint validate packages/api
```

Phase 2: add `configs.mode = "nearest"` and keep `"single"` as the version 1
default.

Phase 3: consider a version 2 default change only after docs, examples, and
warnings have made the behavior familiar.

During phase 2, docs should call out that nested configs are opt-in for parent
directory runs. That avoids silently changing large repositories where nested
`.dollarlint.toml` files were previously only validated as data.

## Issue reporting

Nested config support should make config provenance visible in machine-readable
output eventually, even if text output stays compact.

Potential additions:

- `FileResult.ConfigPath`
- `Warning.Source` values that include the config path for config-derived
  warnings.
- Verbose text output that can show which config selected a schema association
  or ignore rule.

These fields are not required for the first implementation, but they would make
nearest-mode debugging much easier.

## Test plan

Core tests:

- Single mode keeps today's behavior with nested `.dollarlint.toml` files.
- Nearest mode applies a child config to files under the child directory.
- A child config does not inherit root associations unless `extends` is set.
- A child config with `extends` inherits root fetch policy and associations.
- Child associations override parent associations for matching child files.
- Config-authored globs are matched relative to the config directory.
- Relative schema paths in associations resolve relative to the config
  directory.
- In-file `$schema` paths still resolve relative to the document.
- `--config` uses one explicit config and suppresses nested config discovery.
- CLI associations and fetch flags apply to all effective configs.
- Extends cycles fail before validation starts.
- Missing or invalid nested configs fail with a clear config-path error.

Regression tests:

- Built-in `.dollarlint.toml` schema validation still applies to discovered
  config files.
- `schemas.requireCoverage` works per effective config.
- Catalog failure behavior remains unchanged in single mode.
- Nested `.gitignore` handling remains independent of nested config handling.
- JSON, text, and SARIF outputs still count discovered, skipped, failed, and
  ignored files consistently.

## Future extensions

Possible later improvements:

- `dollarlint config inspect <file>` to print the effective config for a file.
- `--config-mode single|nearest` as a diagnostic or migration override.
- Warnings for nested configs found during single mode, gated behind verbose
  output.
- Rule provenance in ignored issue output.
- A repository-wide config graph cache for large monorepos.

These should be added only when they solve a concrete debugging or adoption
problem. The first implementation should stay focused on predictable per-file
config resolution.
