package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GcpSaKeySpec struct {
	Replicas int `json:"replicas"`
	SecretName string `json:"sercretName"`
	SecretDataKey string `json:"secretDataKey"`
	GoogleSaName  string `json:"gcpSaName"`
	OlderThanDays int `json:"olderThanDays"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GcpSaKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GcpSaKeySpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GcpSaKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []GcpSaKey `json:"items"`
}