# Gateway API

This example walks you through how to setup a openZro Gateway API and expose Nginx through the openZro proxy service.

Build image locally and load it into Kind.
```shell
make docker-build IMG=docker.io/openzro/openzro-operator:dev
kind load docker-image docker.io/openzro/openzro-operator:dev
```

Install the Gateway API CRDs.

```shell
kubectl apply --server-side -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.0/experimental-install.yaml
```

Create openZro namespace and API key secret.

```shell
kubectl create namespace openzro
kubectl -n openzro create secret generic openzro-mgmt-api-key --from-literal OZ_API_KEY=${OPENZRO_API_KEY}
```

Install the Kubernetes Operator. Make sure to use the customized values to enable Gateway API support. This assumes you have already created a secret containing a openZro API key.

```shell
helm upgrade --install --create-namespace -f ./examples/gateway-api/values.yaml -n openzro openzro-operator ./helm/openzro-operator
```

Create the gateway along with the routing peer. This will deploy openZro clients that route traffic into the cluster.

```shell
kubectl apply -f ./examples/gateway-api/gateway.yaml
```

Deploy the test Nginx application along with a HTTPRoute. The HTTPRoute will expose the service through openZros public proxy.

```shell
kubectl apply -f ./examples/gateway-api/nginx.yaml
```

Expose the Kubernetes API server service as a network resource in openZro.

```shell
kubectl apply -f ./examples/gateway-api/kubernetes.yaml
```
