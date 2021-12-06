package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type GCPSaKeySpec struct {
	GcpSaName string `json:"gcpSaName"`
	SercretName string `json:"sercretName"`
	SecretDataKey string `json:"secretDataKey"`
	OlderThanDays int `json:"olderThanDays"`
	GoogleProject string `json:"googleProject"`
}

type GCPSaKeyDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GCPSaKeySpec `json:"spec"`
}


type GCPSaKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []GCPSaKeyDefinition `json:"items"`
}

// DeepCopyInto copies all properties of this object into another object of the
// same type that is provided as a pointer.
func (in *GCPSaKeyDefinition) DeepCopyInto(out *GCPSaKeyDefinition) {
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = in.ObjectMeta
	out.Spec = GCPSaKeySpec{
		GcpSaName: in.Spec.GcpSaName,
		SercretName: in.Spec.SercretName,
		SecretDataKey: in.Spec.SecretDataKey,
		OlderThanDays: in.Spec.OlderThanDays,
		GoogleProject: in.Spec.GoogleProject,
	}
}

// DeepCopyObject returns a generically typed copy of an object
func (in *GCPSaKeyDefinition) DeepCopyObject() runtime.Object {
	out := GCPSaKeyDefinition{}
	in.DeepCopyInto(&out)

	return &out
}


// DeepCopyObject returns a generically typed copy of an object
func (in *GCPSaKeyList) DeepCopyObject() runtime.Object {
	out := GCPSaKeyList{}
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta

	if in.Items != nil {
		out.Items = make([]GCPSaKeyDefinition, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}

	return &out
}