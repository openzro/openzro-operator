# openZro Kubernetes Operator

The openZro Kubernetes Operator automates the provisioning of openZro network access for services running in your cluster.

## Documentation

- [Getting Started](/docs/getting-started.md)
- [Usage](/docs/usage.md)
- [API Reference](/docs/api-reference.md)

## How It Works

A `NetworkRouter` registers a openZro router peer for a given DNS zone in your cluster.

```yaml
apiVersion: openzro.io/v1alpha1
kind: NetworkRouter
metadata:
  name: prod
  namespace: openzro
spec:
  dnsZoneRef:
    name: prod.company.internal
```

A `NetworkResource` then exposes a Kubernetes service through that router to one or more openZro groups.

```yaml
apiVersion: openzro.io/v1alpha1
kind: NetworkResource
metadata:
  name: nginx
  namespace: default
spec:
  networkRouterRef:
    name: prod
    namespace: openzro
  serviceRef:
    name: nginx
  groups:
    - name: All
```
