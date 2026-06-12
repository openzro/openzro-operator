package controller

// Shared string constants used across multiple controllers. The
// values were lifted to this file when goconst started flagging
// them as repeated literals; the trade-off (one indirection vs.
// a string typo silently mis-finalizing a resource) tips toward
// the constant.

const (
	// FinalizerResourceCleanup is set on cluster scope resources we
	// reconcile (Services, NetworkResources) so their teardown waits
	// for the controller to clean up the openZro side before the
	// k8s object disappears.
	FinalizerResourceCleanup = "openzro.io/cleanup"

	// FinalizerGroupCleanup gates teardown of objects whose lifecycle
	// is coupled to an openZro Group (peers and routing-peers carry
	// it so the group survives long enough to migrate memberships).
	FinalizerGroupCleanup = "openzro.io/group-cleanup"

	// FinalizerRoutingPeerCleanup is the OZRoutingPeer-specific
	// teardown gate; pairs with FinalizerGroupCleanup on the
	// underlying peer object.
	FinalizerRoutingPeerCleanup = "openzro.io/routing-peer-cleanup"

	// LabelAppKubernetesName is the standard Kubernetes
	// recommended-label key used to identify the workload that
	// owns a pod/service (routing-peer here).
	LabelAppKubernetesName = "app.kubernetes.io/name"

	// LabelValueRouter is the workload identifier the routing-peer
	// reconciler stamps on pods and services it owns.
	LabelValueRouter = "openzro-router"

	// Kind* are the CRD Kind strings used inside owner references
	// the controllers create. The value must match the CRD's
	// `spec.names.kind`; the constants exist so a rename happens
	// in one place.
	KindOZRoutingPeer = "OZRoutingPeer"
	KindOZResource    = "OZResource"

	// SecretKeySetupKey is the field name inside the Secret the
	// operator materializes for a routing-peer; the pod reads its
	// bootstrap setup key from this entry (and the env-var injection
	// references the same name as the SecretKeyRef.Key).
	SecretKeySetupKey = "setupKey"
)
