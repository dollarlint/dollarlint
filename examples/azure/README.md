# Azure ARM examples

These examples validate Azure Resource Manager deployment templates against the
official ARM deployment schema.

```sh
dollarlint validate ./examples/azure --locations
```

That command validates two passing templates and one intentionally invalid
template. To run only the passing ARM examples:

```sh
dollarlint validate ./examples/azure \
  --include storage-account.json \
  --include nested-deployment.json \
  --locations
```

The templates intentionally use real `https://schema.management.azure.com`
schema URLs so they exercise dollarlint's remote schema fetching and Azure ARM
resource pruning behavior.
