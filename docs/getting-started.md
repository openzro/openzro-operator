# Getting Started

## Prerequisites

- A openZro [service user access token](https://docs.openzro.io/manage/public-api).
- Access to Kubernetes cluster.
- Kubectl and Helm installed locally.

## Steps

Add the Helm repository.

```sh
helm repo add openzro https://openzro.github.io/openzro-operator
```

Install cert-manager, it is recommended so the Kubernetes API can communicate with the operator's admission webhooks. Skip this step if you already have cert-manager installed.

```sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.0/cert-manager.yaml
```

Create the openZro namespace and API secret. The operator needs a openZro personal access token to authenticate with the openZro Management API.

```sh
kubectl create namespace openzro
kubectl -n openzro create secret generic openzro-mgmt-api-key --from-literal=OZ_API_KEY=${ACCESS_TOKEN}
```

Install the openZro operator.

```sh
helm install openzro-operator openzro/openzro-operator --create-namespace --namespace openzro
```

Verify the installation. All pods should be in a `Running` state before continuing.

```sh
kubectl get pods -n openzro
```

Once the operator is running, see the [usage guide](/docs/usage.md) to start exposing services to your openZro network.
