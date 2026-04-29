# openZro Kubernetes Operator

The openZro Kubernetes Operator reconciles openZro domain objects
(peers, groups, policies, setup keys, network resources, network
routers) from Kubernetes manifests against an openZro management
server. It lets you GitOps your openZro account just like the rest
of your cluster.

**Current release**: `v0.3.2-alpha.1` — first openZro-fork release
on top of upstream BSD-3 `netbirdio/kubernetes-operator@v0.3.1`.
Image at `ghcr.io/openzro/openzro-operator:0.3.2-alpha.1`
(multi-arch: linux/amd64 + linux/arm64).

For the architecture rationale, see
[ADR-0008](https://github.com/openzro/openzro/blob/main/docs/adr/0008-kubernetes-helm-operator.md)
in the core repo. For the fork-point rebrand details, see
[`FORK.md`](FORK.md).

## Install

The operator ships as a Helm chart published from
[`openzro/helms`](https://github.com/openzro/helms) (the
single-source-of-truth helm repo for everything openZro):

```bash
# 1. Add the openZro helm repo (gh-pages → Cloudflare-fronted)
helm repo add openzro https://openzro.github.io/helms
helm repo update

# 2. Issue a Personal Access Token in the openZro dashboard:
#    Settings → Users → admin → Personal Access Tokens → Generate
PAT="oz_pat_..."

# 3. Wire the PAT into a Kubernetes secret
kubectl -n openzro create secret generic openzro-operator-mgmt \
  --from-literal=managementApiUrl=https://openzro.example.com \
  --from-literal=managementApiToken="$PAT"

# 4. Install the operator chart (current 0.3.2-alpha.1)
helm install openzro-operator openzro/openzro-operator -n openzro \
  --set managementApiSecret=openzro-operator-mgmt
```

For the full deployment walk-through (control plane + Gateway API +
operator + first CRD instance) see
[`docs/operator/k8s-deployment-guide.md`](https://github.com/openzro/openzro/blob/main/docs/operator/k8s-deployment-guide.md)
in the core repo.

## CRDs

The operator ships five CustomResourceDefinitions under the
`openzro.io/v1` API group:

| Kind | Purpose |
|---|---|
| `OZGroup` | Reconcile a peer group in the openZro account |
| `OZPolicy` | Reconcile an access policy (rules + actions) |
| `OZSetupKey` | Provision setup keys for new peers to register |
| `OZResource` | Mark a Kubernetes Service as an openZro network resource |
| `OZRoutingPeer` | Register an openZro router peer for a DNS zone |

Plus two `v1alpha1` CRDs (`Group`, `SetupKey`, `NetworkResource`,
`NetworkRouter`) — the alpha forms are kept while we figure out the
v1 promotion criteria.

### Quick example: provision a group

```yaml
apiVersion: openzro.io/v1
kind: OZGroup
metadata:
  name: backend-team
  namespace: openzro
spec:
  name: "Backend Team"
```

```bash
kubectl apply -f my-group.yaml
kubectl get ozgroups
# NAME            STATUS
# backend-team    Reconciled
```

### Quick example: expose a Service via a network resource

A `NetworkRouter` registers an openZro router peer for a given DNS
zone in your cluster:

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

A `NetworkResource` then exposes a Kubernetes service through that
router to one or more openZro groups:

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

> **Known limitation (2026-04-29)**: The `NetworkResource` and
> `HTTPRoute` controllers depend on DNS Zones + reverse-proxy
> Services API surface that's currently stubbed in the openZro core
> (Tier-1 types exist in `management/server/http/api/`, but
> server-side handlers are deferred — see
> [ADR-0008 Stage 3](https://github.com/openzro/openzro/blob/main/docs/adr/0008-kubernetes-helm-operator.md)).
> CRD instances apply cleanly; reconciliation against a real openZro
> server returns 404 until Tier-2 ships. The native CRDs (OZGroup,
> OZPolicy, OZSetupKey, OZRoutingPeer) reconcile end-to-end today.

## Documentation

- [Getting Started](docs/getting-started.md)
- [Usage](docs/usage.md)
- [API Reference](docs/api-reference.md)

## Development

```bash
# Build + test (uses envtest binaries — `make setup-envtest` first run)
make test

# Local dev against a sibling openzro/openzro checkout
go mod edit -replace=github.com/openzro/openzro=../openzro
go build ./...
```

`make help` lists every available target.

105/105 controller + webhook tests pass against the v0.53.1-alpha.1
core release pinned in `go.mod`.

## Issues / contributions

- Operator bugs / CRD reconciler behavior: file here
- Helm chart packaging: [`openzro/helms`](https://github.com/openzro/helms)
- Management server / API surface: [`openzro/openzro`](https://github.com/openzro/openzro)

## License

[BSD 3-Clause](LICENSE) — preserved verbatim from the upstream
`netbirdio/kubernetes-operator@v0.3.1` fork point.
