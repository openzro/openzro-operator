package v1

import (
	"github.com/openzro/openzro-operator/internal/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OZRoutingPeerSpec defines the desired state of OZRoutingPeer.
type OZRoutingPeerSpec struct {
	// +optional
	Replicas *int32 `json:"replicas"`
	// +optional
	Resources corev1.ResourceRequirements `json:"resources"`
	// +optional
	Labels map[string]string `json:"labels"`
	// +optional
	Annotations map[string]string `json:"annotations"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector"`
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations"`
	// +optional
	Volumes []corev1.Volume `json:"volumes"`
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts"`
	// +optional
	Privileged *bool `json:"privileged,omitempty"`
	// Ephemeral sets the SetupKey Ephemeral flag for the peer
	// registration in openZro. Default: false. Set to true ONLY if
	// you accept that peers get cleaned up 10 min after any
	// disconnect (transient gRPC blips count) — appropriate for
	// short-lived workloads. For typical long-lived routing peer
	// pods, leave at false: pod restarts produce stale peer entries
	// that the operator GC's at OZRoutingPeer delete time, but the
	// running peer survives transient blips.
	// +optional
	Ephemeral *bool `json:"ephemeral,omitempty"`
	// ExemptFromAdmission adds this peer's auto-group to the
	// account's AdmissionExemptGroups so the routing peer pods
	// bypass any posture-check based admission gate. Default: true.
	// Routing peers are headless server-side workloads (cloud VMs,
	// K8s pods) that cannot run MDM/EDR agents, so the admission
	// posture check would otherwise lock the routing infrastructure
	// out of its own mesh. Set to false ONLY if you have a
	// posture-check that is meaningful for headless gateway peers.
	// +optional
	ExemptFromAdmission *bool `json:"exemptFromAdmission,omitempty"`
}

// OZRoutingPeerStatus defines the observed state of OZRoutingPeer.
type OZRoutingPeerStatus struct {
	// +optional
	NetworkID *string `json:"networkID"`
	// +optional
	SetupKeyID *string `json:"setupKeyID"`
	// +optional
	RouterID *string `json:"routerID"`
	// +optional
	Conditions []OZCondition `json:"conditions,omitempty"`
}

// Equal returns if OZRoutingPeerStatus is equal to this one
func (a OZRoutingPeerStatus) Equal(b OZRoutingPeerStatus) bool {
	return a.NetworkID == b.NetworkID &&
		a.SetupKeyID == b.SetupKeyID &&
		a.RouterID == b.RouterID &&
		util.Equivalent(a.Conditions, b.Conditions)
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OZRoutingPeer is the Schema for the ozroutingpeers API.
type OZRoutingPeer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OZRoutingPeerSpec   `json:"spec,omitempty"`
	Status OZRoutingPeerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OZRoutingPeerList contains a list of OZRoutingPeer.
type OZRoutingPeerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OZRoutingPeer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OZRoutingPeer{}, &OZRoutingPeerList{})
}
