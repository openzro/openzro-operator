# Usage

## Exposing Services

The operator exposes Kubernetes services to your openZro network by combining two resources, a `NetworkRouter` and a `NetworkResource`.

### NetworkRouter

A `NetworkRouter` creates a network in openZro and deploys routing peer pods in the cluster. These pods are configured as routing peers for the network, handling traffic between openZro clients and services running in the cluster.

Before creating a `NetworkRouter`, you must first create a custom DNS zone in the [openZro dashboard](https://docs.openzro.io/manage/dns/custom-zones). The DNS zone must exist before the operator can register it.

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

### NetworkResource

A `NetworkResource` exposes a Kubernetes service in openZro by creating a matching resource in the routers network. The cluster IP of the service will be used as the resource IP. A record in the routers zone will also be created using the name and namespace of the service. The following example creates an nignx deployment and exposes the service with the record `nginx.default.prod.company.internal`.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: default
  labels:
    app: nginx
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - image: nginx
        imagePullPolicy: Always
        name: nginx
---
apiVersion: v1
kind: Service
metadata:
  name: nginx
  namespace: default
  labels:
    app: nginx
spec:
  type: ClusterIP
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: 80
  selector:
    app: nginx
---
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

Members of the `All` openZro group can now reach the nginx service at `nginx.default.prod.company.internal` through the openZro network.

## Client Sidecar

In certain situations we may want to have a pod act like a peer in the openZro network instead of exposing it through a routing peer. In these cases a openZro client container has to be added as a sidecar to the pod.

Sidecars are appended to pods when created if they match the selector of a sidecar profile. A sidecar profile defines the configuration of the sidecar, like the setup key to be used along with other parameters. An empty selector will match with all pods in the namespace. The sidecar profile needs to be created first before any pod is created.

```yaml
apiVersion: openzro.io/v1alpha1
kind: SetupKey
metadata:
  name: sidecar
  namespace: default
spec:
  name: sidecar
  ephemeral: true
---
apiVersion: openzro.io/v1alpha1
kind: SidecarProfile
metadata:
  name: test
  namespace: default
spec:
  setupKeyRef:
    name: sidecar
  podSelector:
    matchLabels:
      app: ubuntu
```

When a pod matching the selector is created it will receive a openzro sidecar container.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: ubuntu
  namespace: default
  labels:
    app: ubuntu
spec:
  containers:
  - name: ubuntu
    image: ubuntu:latest
    command: ["sleep", "infinity"]
```

Once both containers have started the pod should show up like a peer in the openZro dashboard.
