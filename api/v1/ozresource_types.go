package v1

import (
	"maps"

	"github.com/openzro/openzro-operator/internal/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OZResourceSpec defines the desired state of OZResource.
type OZResourceSpec struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	NetworkID string `json:"networkID"`
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`
	// +kubebuilder:validation:items:MinLength=1
	Groups []string `json:"groups"`
	// +optional
	PolicyName string `json:"policyName,omitempty"`
	// +optional
	PolicySourceGroups []string `json:"policySourceGroups,omitempty"`
	// +optional
	PolicyFriendlyName map[string]string `json:"policyFriendlyName,omitempty"`
	// +optional
	TCPPorts []int32 `json:"tcpPorts,omitempty"`
	// +optional
	UDPPorts []int32 `json:"udpPorts,omitempty"`
}

// Equal returns if OZResource is equal to this one
func (a OZResourceSpec) Equal(b OZResourceSpec) bool {
	return a.Name == b.Name &&
		a.NetworkID == b.NetworkID &&
		a.Address == b.Address &&
		util.Equivalent(a.Groups, b.Groups) &&
		a.PolicyName == b.PolicyName &&
		util.Equivalent(a.TCPPorts, b.TCPPorts) &&
		util.Equivalent(a.UDPPorts, b.UDPPorts) &&
		util.Equivalent(a.PolicySourceGroups, b.PolicySourceGroups)
}

// OZResourceStatus defines the observed state of OZResource.
type OZResourceStatus struct {
	// +optional
	NetworkResourceID *string `json:"networkResourceID,omitempty"`
	// +optional
	PolicyName *string `json:"policyName,omitempty"`
	// +optional
	TCPPorts []int32 `json:"tcpPorts,omitempty"`
	// +optional
	UDPPorts []int32 `json:"udpPorts,omitempty"`
	// +optional
	Groups []string `json:"groups,omitempty"`
	// +optional
	PolicySourceGroups []string `json:"policySourceGroups,omitempty"`
	// +optional
	PolicyFriendlyName map[string]string `json:"policyFriendlyName,omitempty"`
	// +optional
	Conditions []OZCondition `json:"conditions,omitempty"`
	// +optional
	PolicyNameMapping map[string]string `json:"policyNameMapping"`
}

// Equal returns if OZResourceStatus is equal to this one
func (a OZResourceStatus) Equal(b OZResourceStatus) bool {
	return a.NetworkResourceID == b.NetworkResourceID &&
		a.PolicyName == b.PolicyName &&
		util.Equivalent(a.TCPPorts, b.TCPPorts) &&
		util.Equivalent(a.UDPPorts, b.UDPPorts) &&
		util.Equivalent(a.Groups, b.Groups) &&
		util.Equivalent(a.Conditions, b.Conditions) &&
		util.Equivalent(a.PolicySourceGroups, b.PolicySourceGroups) &&
		maps.Equal(a.PolicyFriendlyName, b.PolicyFriendlyName) &&
		maps.Equal(a.PolicyNameMapping, b.PolicyNameMapping)
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OZResource is the Schema for the ozresources API.
type OZResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OZResourceSpec   `json:"spec,omitempty"`
	Status OZResourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OZResourceList contains a list of OZResource.
type OZResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OZResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OZResource{}, &OZResourceList{})
}
