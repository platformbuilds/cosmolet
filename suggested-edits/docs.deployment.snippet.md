
## Fast path: generate configs
```bash
./tools/gen-values.sh
helm upgrade --install cosmolet ./charts/cosmolet -n kube-system -f generated/helm-values.yaml
```
