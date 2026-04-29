/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OZConditionType is a valid value for PodCondition.Type
type OZConditionType string

// These are built-in conditions of pod. An application may use a custom condition not listed here.
const (
	// OZSetupKeyReady indicates whether OZSetupKey is valid and ready to use.
	OZSetupKeyReady OZConditionType = "Ready"
)

// OZSetupKeySpec defines the desired state of OZSetupKey.
type OZSetupKeySpec struct {
	// SecretKeyRef is a reference to the secret containing the setup key
	// +kubebuilder:validation:XValidation:rule="self.name.size() > 0",reason="FieldValueRequired",message="secret name needs to be set",fieldPath=".name"
	SecretKeyRef corev1.SecretKeySelector `json:"secretKeyRef"`
	// ManagementURL optional, override operator management URL
	ManagementURL string `json:"managementURL,omitempty"`
	// Volumes optional, additional volumes for openZro container
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`
	// VolumeMounts optional, additional volumeMounts for openZro container
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`
}

// OZSetupKeyStatus defines the observed state of OZSetupKey.
type OZSetupKeyStatus struct {
	// +optional
	Conditions []OZCondition `json:"conditions,omitempty"`
}

// OZCondition defines a condition in OZSetupKey status.
type OZCondition struct {
	// Type is the type of the condition.
	Type OZConditionType `json:"type"`
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// OZConditionTrue returns default true condition
func OZConditionTrue() []OZCondition {
	return []OZCondition{
		{
			Type:               OZSetupKeyReady,
			LastProbeTime:      metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Status:             corev1.ConditionTrue,
		},
	}
}

// OZConditionFalse returns default false condition
func OZConditionFalse(reason, msg string) []OZCondition {
	return []OZCondition{
		{
			Type:               OZSetupKeyReady,
			LastProbeTime:      metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Status:             corev1.ConditionFalse,
			Reason:             reason,
			Message:            msg,
		},
	}
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OZSetupKey is the Schema for the ozsetupkeys API.
type OZSetupKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OZSetupKeySpec   `json:"spec,omitempty"`
	Status OZSetupKeyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OZSetupKeyList contains a list of OZSetupKey.
type OZSetupKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OZSetupKey `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OZSetupKey{}, &OZSetupKeyList{})
}
