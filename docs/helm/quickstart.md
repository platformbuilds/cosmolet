
# Helm Quickstart

```bash
# Choose a values file in docs/examples/helm/
helm upgrade --install cosmolet ./charts/cosmolet -n kube-system   -f docs/examples/helm/values-single-tor.yaml
```

See the **values reference** and more variants in: [values.md](values.md) and [examples/helm](../examples/helm/).
