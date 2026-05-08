# Schema Alterations

## Problem

Some teams need to validate documents against a vendor schema while also
enforcing local requirements that the vendor schema does not know about.

The motivating case is Azure ARM templates. The upstream ARM schema rejects
unknown root properties, but some internal workflows require an extra company
property. Today this can be handled with an `ignore` rule for the
`additionalProperties` failure, but that is only a suppression. It cannot express
that the property is valid in this repository, required by local policy, and
itself subject to validation.

Schema alterations are intended to model that local contract directly.

## Goals

- Let users extend validation semantics without authoring arbitrary JSON Patch
  files against vendor schemas.
- Keep the public config focused on validation intent rather than schema
  internals.
- Support file-scoped local rules, because the same vendor schema may need
  different local policy in different parts of a repository.
- Preserve normal vendor schema validation for everything outside the explicit
  local rule.
- Allow local rules to add failures, not only remove failures.
- Keep implementation testable and isolated from the raw schema cache.

## Non-goals

- Do not provide a general-purpose JSON Schema patching or bundling system.
- Do not require users to know schema-document JSON Pointer paths inside vendor
  schemas.
- Do not fully resolve or inline `$ref` graphs before applying changes.
- Do not make alterations a replacement for `ignore` rules.

## Proposed config

Start with a single alteration kind:

```toml
[[schemas.alterations]]
kind = "objectProperty"
files = ["infra/**/*.json"]
schema = "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#"
instancePath = "/"
property = "x-company"
required = true
valueSchema = "./schemas/x-company.schema.json"
```

Field meanings:

- `kind`: the typed alteration operation. Version one supports
  `"objectProperty"`.
- `files`: optional list of file glob patterns matched against discovered
  relative paths.
- `schema`: optional root schema URI selector. If present, it is compared after
  the document schema URI is resolved.
- `instancePath`: dollarlint's normalized instance pointer into the input
  document, not into the schema document. This uses the same root convention as
  current output and source maps: `"/"` means the document root, and
  `"/resources/0"` means the first item in `resources`.
- `property`: object property name governed by the alteration.
- `required`: whether the property must be present.
- `valueSchema`: optional schema URI used to validate the property's value.

At least one of `files` or `schema` must be present. If both are present, both
must match.

## Semantics

An `objectProperty` alteration says:

> For documents selected by this rule, treat `property` at `instancePath` as a
> valid local extension property. Optionally require it and validate its value.

This is intentionally different from an `ignore` rule. An ignore says "the base
schema reported a failure, but do not count it." An alteration says "this is part
of the intended local schema contract."

For the example above:

- ARM template validation still applies normally.
- A root `x-company` property is not reported as an unknown-property error.
- Missing root `x-company` is reported as an alteration validation issue.
- If `valueSchema` is set, the value at `/x-company` is validated against that
  schema and failures are reported as alteration validation issues.

## Relationship to ignores

There is intentional overlap with `ignore` for the narrow case of allowing an
extra property. The boundary should be:

- Use `ignore` for waivers, known defects, temporary exceptions, and legacy
  documents.
- Use `schemas.alterations` for local policy that should be enforced.

Alterations become meaningfully different from ignores when they require the
property or validate its value. This feature should be documented as a way to
add a local contract, not as a more complicated suppression syntax.

## Matching rules

Alterations are evaluated per document after schema discovery and URI
resolution.

Rules:

- `files` matches `Document.RelativePath` using the same pattern behavior as
  schema associations and ignores.
- `schema` matches the resolved root schema URI. Fragment normalization should
  follow the same rules used by schema loading.
- `files + schema` is an AND.
- Matching alterations are applied in config order.

## Implementation shape

The first version does not need to mutate schema documents. It can be a layered
validation pass:

1. Discover and parse documents as today.
2. Resolve the root schema URI as today.
3. Compute the alterations that match the document.
4. Run normal base schema validation.
5. Suppress only base-schema issues that are exactly covered by matching
   `objectProperty` alterations, such as an `additionalProperties` or
   `unevaluatedProperties` issue for the configured property at the configured
   instance path.
6. Run alteration validation against the original document:
   - resolve `instancePath`,
   - verify the target is an object when needed,
   - check `required`,
   - validate the property value against `valueSchema` if configured.
7. Add alteration failures as normal issues.

This keeps the raw schema cache unchanged. It also avoids a patched-schema cache
key problem where two files using the same schema could need different variants.

If issue-level suppression from step 5 proves too dependent on validator error
details, a fallback design is to run base validation against a cloned document
where covered extension properties are temporarily removed. That fallback should
be used carefully because it can hide base-schema constraints if someone
configures an alteration for a property the base schema already understands.

## Issue reporting

Alteration failures should look like validation issues, but they need stable
metadata so text, JSON, and SARIF outputs can distinguish them.

Suggested fields:

- `Keyword`: `alterationRequired`, `alterationType`, or
  `alterationValueSchema`.
- `InstanceLocation`: the JSON Pointer to the affected property or object.
- `Property`: the configured property name when applicable.
- `Schema`: the document's root schema URI, or `valueSchema` for value-schema
  failures if that is more useful in verbose output.

Messages should be direct:

- `property "x-company" is required by schema alteration`
- `property "x-company" must be an object`
- `property "x-company" does not match alteration value schema: ...`

## Config schema/API changes

- Add `Alterations []SchemaAlteration` to `SchemaConfig`.
- Add `SchemaAlteration` with fields matching the config above.
- Add validation in `validateConfigValues`:
  - supported `kind`,
  - at least one selector,
  - non-empty `property`,
  - valid normalized `instancePath`,
  - valid `valueSchema` URI syntax if feasible before document context.
- Update `schemas/dollarlint.schema.json` and the embedded config schema.
- Expose the new type through top-level type aliases in `types.go`.

## Test plan

Core tests:

- A matching ARM-like document with the local property passes even when the base
  schema disallows additional properties.
- A matching document missing a required local property fails.
- A matching document with an invalid local property value fails against
  `valueSchema`.
- A non-matching file using the same root schema still reports the original
  additional-property failure.
- Two files using the same schema, one matching and one not, are independent
  regardless of validation order.
- `files + schema` uses AND semantics.
- `schema`-only alterations apply to every file using that root schema.
- `files`-only alterations apply to the selected files' root schemas.
- Invalid alteration config fails before validation starts.

Regression tests:

- Existing ignores still work and keep their current output.
- Existing schema associations, catalog matches, and built-in config schema
  validation are unaffected.
- Source locations still point to the original document property for alteration
  issues.
- JSON and SARIF output include alteration issues consistently.

## Future extensions

Possible future alteration kinds:

- `enumValue`: allow or require additional enum values for a selected instance
  path.
- `propertyValue`: require a property to equal or match a configured constant or
  pattern.
- `objectProperties`: declare several related extension properties in one rule.

These should be added only when there is a clear user-facing validation intent.
Avoid introducing an alteration kind that is merely a disguised JSON Patch
operation.
