package v1beta1

import (
	"encoding"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type GCPSaKeySpec struct {
	GoogleServiceAccount GoogleServiceAccount `json:"googleServiceAccount"`
	Secret               Secret               `json:"secret"`
	VaultReplications    []VaultReplication   `json:"vaultReplications"`
	KeyRotation          KeyRotation          `json:"keyRotation"`
}

type GoogleServiceAccount struct {
	Name    string `json:"name"`
	Project string `json:"project"`
}

type Secret struct {
	Name        string `json:"name"`
	PemKeyName  string `json:"pemKeyName"`
	JsonKeyName string `json:"jsonKeyName"`
	// ClientSecretKeyName Optional field to specify the key name for an azure client secret
	ClientSecretKeyName string `json:"clientSecretKeyName,omitempty"`
}

type KeyRotation struct {
	RotateAfter        int  `json:"rotateAfter"`
	DeleteAfter        int  `json:"deleteAfter"`
	DisableAfter       int  `json:"disableAfter"`
	IgnoreUsageMetrics bool `json:"ignoreUsageMetrics"`
}

type VaultReplication struct {
	Path   string                 `json:"path"`
	Format VaultReplicationFormat `json:"format"`
	Key    string                 `json:"key"`
}

type VaultReplicationFormat int64

const (
	Map VaultReplicationFormat = iota
	JSON
	Base64
	PEM
	PlainText
)

// verify format implements expected interfaces
var _ encoding.TextUnmarshaler = (*VaultReplicationFormat)(nil)
var _ encoding.TextMarshaler = (VaultReplicationFormat)(0)

func (v VaultReplicationFormat) String() string {
	switch v {
	case Map:
		return "map"
	case JSON:
		return "json"
	case Base64:
		return "base64"
	case PEM:
		return "pem"
	case PlainText:
		return "plainText"
	default:
		return "unknown"
	}
}

func (v VaultReplicationFormat) MarshalText() ([]byte, error) {
	switch v {
	case Map, JSON, Base64, PEM, PlainText:
		return []byte(v.String()), nil
	default:
		return nil, fmt.Errorf("unknown replication format: %#v", v)
	}
}

func (v *VaultReplicationFormat) UnmarshalText(data []byte) error {
	s := string(data)
	switch s {
	case "map":
		*v = Map
		return nil
	case "json":
		*v = JSON
		return nil
	case "base64":
		*v = Base64
		return nil
	case "pem":
		*v = PEM
		return nil
	case "plainText":
		*v = PlainText
		return nil
	default:
		return fmt.Errorf("unknown replication format: %q", s)
	}
}

type GcpSaKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GCPSaKeySpec `json:"spec"`
}

type GCPSaKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []GcpSaKey `json:"items"`
}

// DeepCopyInto copies all properties of this object into another object of the
// same type that is provided as a pointer.
func (in *GcpSaKey) DeepCopyInto(out *GcpSaKey) {
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = in.ObjectMeta
	out.Spec = GCPSaKeySpec{
		GoogleServiceAccount: in.Spec.GoogleServiceAccount,
		Secret:               in.Spec.Secret,
		KeyRotation:          in.Spec.KeyRotation,
	}
}

// DeepCopyObject returns a generically typed copy of an object
func (in *GcpSaKey) DeepCopyObject() runtime.Object {
	out := GcpSaKey{}
	in.DeepCopyInto(&out)

	return &out
}

// DeepCopyObject returns a generically typed copy of an object
func (in *GCPSaKeyList) DeepCopyObject() runtime.Object {
	out := GCPSaKeyList{}
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]GcpSaKey, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return &out
}
