package v1

import (
	"github.com/openzro/openzro-operator/internal/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OZPolicySpec defines the desired state of OZPolicy.
type OZPolicySpec struct {
	// Name Policy name
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +optional
	Description string `json:"description,omitempty"`
	// +optional
	// +kubebuilder:validation:items:MinLength=1
	SourceGroups []string `json:"sourceGroups,omitempty"`
	// +optional
	// +kubebuilder:validation:items:MinLength=1
	DestinationGroups []string `json:"destinationGroups,omitempty"`
	// +optional
	// +kubebuilder:validation:items:Enum=tcp;udp
	Protocols []string `json:"protocols,omitempty"`
	// +optional
	// +kubebuilder:validation:items:Minimum=0
	// +kubebuilder:validation:items:Maximum=65535
	Ports []int32 `json:"ports,omitempty"`
	// +optional
	// +default:value=true
	Bidirectional bool `json:"bidirectional"`
}

// OZPolicyStatus defines the observed state of OZPolicy.
type OZPolicyStatus struct {
	// +optional
	TCPPolicyID *string `json:"tcpPolicyID"`
	// +optional
	UDPPolicyID *string `json:"udpPolicyID"`
	// +optional
	LastUpdatedAt *metav1.Time `json:"lastUpdatedAt"`
	// +optional
	ManagedServiceList []string `json:"managedServiceList"`
	// +optional
	Conditions []OZCondition `json:"conditions,omitempty"`
}

// Equal returns if OZPolicyStatus is equal to this one
func (a OZPolicyStatus) Equal(b OZPolicyStatus) bool {
	return a.TCPPolicyID == b.TCPPolicyID &&
		a.UDPPolicyID == b.UDPPolicyID &&
		a.LastUpdatedAt == b.LastUpdatedAt &&
		util.Equivalent(a.ManagedServiceList, b.ManagedServiceList) &&
		util.Equivalent(a.Conditions, b.Conditions)
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// OZPolicy is the Schema for the ozpolicies API.
type OZPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OZPolicySpec   `json:"spec,omitempty"`
	Status OZPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OZPolicyList contains a list of OZPolicy.
type OZPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OZPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OZPolicy{}, &OZPolicyList{})
}
