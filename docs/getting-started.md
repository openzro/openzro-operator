# Getting Started

## Prerequisites

- A openZro [service user access token](https://docs.openzro.io/manage/public-api)
  (Personal Access Token issued from the dashboard, Settings → Users
  → admin → Personal Access Tokens → Generate).
- Access to Kubernetes cluster (1.27+).
- `kubectl` matching your cluster + `helm` v3.12+ installed locally.

## Steps

Add the Helm repository. openZro consolidates all charts under a
single `openzro/helms` repo (control plane + operator + operator
config) — the same one is used regardless of which component you
install:

```sh
helm repo add openzro https://openzro.github.io/helms
helm repo update
```

Install cert-manager. Recommended so the Kubernetes API can communicate
with the operator's admission webhooks. Skip if you already have
cert-manager installed.

```sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.0/cert-manager.yaml
```

Create the openZro namespace and API secret. The operator needs a
openZro Personal Access Token to authenticate with the openZro
Management API.

```sh
kubectl create namespace openzro
kubectl -n openzro create secret generic openzro-mgmt-api-key \
  --from-literal=OZ_API_KEY=${ACCESS_TOKEN}
```

Install the openZro operator (current release `0.3.2-alpha.1` ships
multi-arch image at `ghcr.io/openzro/openzro-operator:0.3.2-alpha.1`):

```sh
helm install openzro-operator openzro/openzro-operator \
  --namespace openzro
```

Verify the installation. All pods should be in a `Running` state
before continuing.

```sh
kubectl get pods -n openzro
```

Once the operator is running, see the [usage guide](usage.md) to
start exposing services to your openZro network.

## Pull-secret caveat (private GHCR packages)

At the time of this writing the openZro container packages on
`ghcr.io` are private by default. If `kubectl get pods -n openzro`
shows `ImagePullBackOff`, create a docker-registry secret with a
PAT that has `read:packages` and reference it via
`imagePullSecrets`:

```sh
kubectl -n openzro create secret docker-registry ghcr-openzro \
  --docker-server=ghcr.io \
  --docker-username=<github-handle> \
  --docker-password=<PAT-with-read:packages>

helm upgrade openzro-operator openzro/openzro-operator -n openzro \
  --reuse-values \
  --set 'operator.imagePullSecrets[0].name=ghcr-openzro'
```

Once the org flips package visibility to Public via the GitHub UI,
the pull-secret becomes optional.
