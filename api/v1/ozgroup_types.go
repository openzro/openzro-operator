package v1

import (
	"github.com/openzro/openzro-operator/internal/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OZGroupSpec defines the desired state of OZGroup.
type OZGroupSpec struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// OZGroupStatus defines the observed state of OZGroup.
type OZGroupStatus struct {
	// +optional
	GroupID *string `json:"groupID"`
	// +optional
	Conditions []OZCondition `json:"conditions,omitempty"`
}

// Equal returns if OZGroupStatus is equal to this one
func (a OZGroupStatus) Equal(b OZGroupStatus) bool {
	return (a.GroupID == b.GroupID || (a.GroupID != nil && b.GroupID != nil && *a.GroupID == *b.GroupID)) && util.Equivalent(a.Conditions, b.Conditions)
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OZGroup is the Schema for the ozgroups API.
type OZGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OZGroupSpec   `json:"spec,omitempty"`
	Status OZGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OZGroupList contains a list of OZGroup.
type OZGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OZGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OZGroup{}, &OZGroupList{})
}
