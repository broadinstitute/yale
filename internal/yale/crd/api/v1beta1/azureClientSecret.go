package v1beta1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

type AzureClientSecretSpec struct {
	AzureServicePrincipal           AzureServicePrincipal            `json:"azureServicePrincipal"`
	Secret                          Secret                           `json:"secret"`
	VaultReplications               []VaultReplication               `json:"vaultReplications"`
	GoogleSecretManagerReplications []GoogleSecretManagerReplication `json:"googleSecretManagerReplications"`
	KeyRotation                     KeyRotation                      `json:"keyRotation"`
}

type AzureServicePrincipal struct {
	TenantID      string `json:"tenantID"`
	ApplicationID string `json:"applicationID"`
}

type AzureClientSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AzureClientSecretSpec `json:"spec"`
}

type AzureClientSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AzureClientSecret `json:"items"`
}

// DeepCopyInto copies all properties of this object into another object of the
// same type that is provided as a pointer.
func (in *AzureClientSecret) DeepCopyInto(out *AzureClientSecret) {
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = in.ObjectMeta
	out.Spec = AzureClientSecretSpec{
		AzureServicePrincipal: in.Spec.AzureServicePrincipal,
		Secret:                in.Spec.Secret,
		KeyRotation:           in.Spec.KeyRotation,
	}
}

// DeepCopyObject returns a generically typed copy of an object
func (in *AzureClientSecret) DeepCopyObject() runtime.Object {
	out := AzureClientSecret{}
	in.DeepCopyInto(&out)

	return &out
}

// DeepCopyObject returns a generically typed copy of an object
func (in *AzureClientSecretList) DeepCopyObject() runtime.Object {
	out := AzureClientSecretList{}
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]AzureClientSecret, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return &out
}

func (g AzureClientSecret) Name() string {
	return g.ObjectMeta.Name
}

func (g AzureClientSecret) Namespace() string {
	return g.ObjectMeta.Namespace
}

func (g AzureClientSecret) SecretName() string {
	return g.Spec.Secret.Name
}

func (g AzureClientSecret) SpecBytes() ([]byte, error) {
	return json.Marshal(g.Spec)
}

func (g AzureClientSecret) VaultReplications() []VaultReplication {
	return g.Spec.VaultReplications
}

func (g AzureClientSecret) GoogleSecretManagerReplications() []GoogleSecretManagerReplication {
	return g.Spec.GoogleSecretManagerReplications
}

func (g AzureClientSecret) APIVersion() string {
	return g.TypeMeta.APIVersion
}

func (g AzureClientSecret) Kind() string {
	return g.TypeMeta.Kind
}

func (g AzureClientSecret) UID() types.UID {
	return g.ObjectMeta.UID
}

func (g AzureClientSecret) Labels() map[string]string {
	return g.ObjectMeta.Labels
}

func (g AzureClientSecret) Secret() Secret {
	return g.Spec.Secret
}
