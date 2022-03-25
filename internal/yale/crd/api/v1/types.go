package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type GCPSaKeySpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	GcpSaName         string `json:"gcpSaName"`
	SecretName        string `json:"secretName"`
	Namespace         string `json:"namespace"`
	PemDataFieldName  string `json:"pemDataFieldName"`
	PrivateKeyDataFieldName     string `json:"privateKeyDataFieldName"`
	OlderThanDays     int    `json:"olderThanDays"`
	GoogleProject     string `json:"googleProject"`
	DaysDisabled     int `json:"daysDisabled"`
	DaysDeauthenticated     int `json:"daysDeauthenticated"`
}

type GCPSaKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GCPSaKeySpec `json:"spec"`
}

type GCPSaKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []GCPSaKey `json:"items"`
}

// DeepCopyInto copies all properties of this object into another object of the
// same type that is provided as a pointer.
func (in *GCPSaKey) DeepCopyInto(out *GCPSaKey) {
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = in.ObjectMeta
	out.Spec = GCPSaKeySpec{
		GcpSaName:     in.Spec.GcpSaName,
		SecretName:    in.Spec.SecretName,
		PrivateKeyDataFieldName: in.Spec.PrivateKeyDataFieldName,
		OlderThanDays: in.Spec.OlderThanDays,
		GoogleProject: in.Spec.GoogleProject,
	}
}

// DeepCopyObject returns a generically typed copy of an object
func (in *GCPSaKey) DeepCopyObject() runtime.Object {
	out := GCPSaKey{}
	in.DeepCopyInto(&out)

	return &out
}

// DeepCopyObject returns a generically typed copy of an object
func (in *GCPSaKeyList) DeepCopyObject() runtime.Object {
	out := GCPSaKeyList{}
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]GCPSaKey, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return &out
}