# Azure ARM examples

These examples validate Azure Resource Manager deployment templates against the
official ARM deployment schema.

```sh
dollarlint validate ./examples/azure --locations
```

The templates intentionally use real `https://schema.management.azure.com`
schema URLs so they exercise dollarlint's remote schema fetching and Azure ARM
resource pruning behavior.
