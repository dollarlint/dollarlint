---
title: Exit codes
description: How dollarlint signals success and failure to the shell.
---

`dollarlint` uses three distinct exit codes so CI systems can differentiate
between data findings and tool problems:

| Code | Meaning                                                                 |
| ---- | ----------------------------------------------------------------------- |
| `0`  | No non-ignored issues. The run succeeded.                               |
| `1`  | Validation, schema loading, or parsing issues were found.               |
| `2`  | CLI or configuration error (bad flags, unreadable config, and similar). |

When `schema.schemaStore.failure = "error"`, SchemaStore catalog
unavailability is treated as a configuration error and exits `2` rather
than `1`.
