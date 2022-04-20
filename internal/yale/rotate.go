package yale

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/client"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	clientv1 "github.com/broadinstitute/yale/internal/yale/crd/clientset/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/policyanalyzer/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	//"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	//"time"
)

// keyAlgorithm what key algorithm to use when creating new Google SA keys
const KEY_ALGORITHM string = "KEY_ALG_RSA_2048"

// keyFormat format to use when creating new Google SA keys
const KEY_FORMAT string = "TYPE_GOOGLE_CREDENTIALS_FILE"

type Yale struct { // Yale config
	gcp   *iam.Service              // GCP IAM API client
	gcpPA *policyanalyzer.Service   // GCP Policy API client
	k8s   kubernetes.Interface      // K8s API client
	crd   clientv1.YaleCRDInterface // K8s CRD API client

	//Function yale will execute

}

type saKeyData struct {
	PrivateKey string `json:"private_key"`
}

type SaKey struct {
	privateKeyData        string
	serviceAccountKeyName string
	serviceAccountName    string
	validAfterTime        string
	disabled              bool
}

// NewYale /* Construct a new Yale Manager */
func NewYale(clients *client.Clients) (*Yale, error) {
	k8s := clients.GetK8s()
	gcp := clients.GetGCP()
	crd := clients.GetCRDs()
	gcpPA := clients.GetGCPPA()

	return &Yale{gcp, gcpPA, k8s, crd}, nil
}

func (m *Yale) RotateKeys() error {
	// Get all GCPSaKey resource
	result, err := m.GetGCPSaKeyList()
	if err != nil {
		return err
	}
	for _, gcpsakey := range result.Items {
		err := m.rotateKey(gcpsakey)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Yale) rotateKey(Gsk apiv1b1.GCPSaKey) error {
	exists, err := m.secretExists(Gsk.Spec.Secret, Gsk.Namespace)
	if err != nil {
		return err
	}
	if !exists {
		return m.CreateSecret(Gsk)
	} else {
		return m.UpdateKey(Gsk.Spec, Gsk.Namespace)
	}
}

func (m *Yale) secretExists(secret apiv1b1.Secret, namespace string) (bool, error) {
	_, err := m.GetSecret(secret, namespace)
	if err == nil {
		return true, nil
	}
	if errors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

// GetGCPSaKeyList Returns list of GSKs
func (m *Yale) GetGCPSaKeyList() (result *apiv1b1.GCPSaKeyList, err error) {
	return m.crd.GcpSaKeys().List(context.Background(), metav1.ListOptions{})
}

// Creates basic annotations for Secret
func createAnnotations(key SaKey) map[string]string {
	return map[string]string{
		"serviceAccountKeyName":       key.serviceAccountKeyName,
		"serviceAccountName":          key.serviceAccountName,
		"validAfterDate":              key.validAfterTime,
		"reloader.stakater.com/match": "true",
	}
}

// CreateSecret Creates a secret for a new GSK resource
func (m *Yale) CreateSecret(Gsk apiv1b1.GCPSaKey) error {
	logs.Info.Printf("Creating secret %s ...", Gsk.Spec.Secret.Name)
	saKey, err := m.CreateSAKey(Gsk.Spec.GoogleServiceAccount.Project, Gsk.Spec.GoogleServiceAccount.Name)

	if err != nil {
		return err
	}
	jsonKey, err := base64.StdEncoding.DecodeString(saKey.privateKeyData)
	if err != nil {
		return err
	}
	saData := saKeyData{}
	err = json.Unmarshal(jsonKey, &saData)
	if err != nil {
		return err
	}

	// Create ownership reference
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents
	var ownerRef = []metav1.OwnerReference{
		{
			APIVersion: Gsk.APIVersion,
			Kind:       Gsk.Kind,
			Name:       Gsk.Name,
			UID:        Gsk.UID,
		},
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       Gsk.Namespace,
			Name:            Gsk.Spec.Secret.Name,
			Labels:          Gsk.Labels,
			Annotations:     createAnnotations(*saKey),
			OwnerReferences: ownerRef,
		},
		StringData: map[string]string{
			Gsk.Spec.Secret.JsonKeyName: string(jsonKey),
			Gsk.Spec.Secret.PemKeyName:  saData.PrivateKey,
		},
		Type: corev1.SecretTypeOpaque,
	}
	_, err = m.k8s.CoreV1().Secrets(Gsk.Namespace).Create(context.TODO(), newSecret, metav1.CreateOptions{})

	if err != nil {
		return err
	}
	return nil
}

//UpdateKey Updates pem data and private key data fields in Secret with new key
func (m *Yale) UpdateKey(GskSpec apiv1b1.GCPSaKeySpec, namespace string) error {
	K8Secret, err := m.GetSecret(GskSpec.Secret, namespace)
	if err != nil {
		return err
	}
	// Annotations are not queryable
	originalAnnotations := K8Secret.GetAnnotations()
	keyIsExpired, err := IsExpired(originalAnnotations["validAfterDate"], GskSpec.KeyRotation.RotateAfter, originalAnnotations["serviceAccountKeyName"])
	if err != nil {
		return err
	}
	if !keyIsExpired {
		return nil
	}

	Key, err := m.CreateSAKey(GskSpec.GoogleServiceAccount.Project, GskSpec.GoogleServiceAccount.Name)
	if err != nil {
		return err
	}
	// Create annotations for new key
	newAnnotations := createAnnotations(*Key)
	// Add expired service account name to new annotation for tracking
	newAnnotations["oldServiceAccountKeyName"] = originalAnnotations["serviceAccountKeyName"]
	K8Secret.Annotations = originalAnnotations // Set the secret's annotations

	saKey, err := base64.StdEncoding.DecodeString(Key.privateKeyData)
	if err != nil {
		return err
	}
	saData := saKeyData{}
	err = json.Unmarshal(saKey, &saData)
	if err != nil {
		return err
	}
	K8Secret.Data[GskSpec.Secret.JsonKeyName] = saKey
	K8Secret.Data[GskSpec.Secret.PemKeyName] = []byte(saData.PrivateKey)
	err = m.UpdateSecret(K8Secret)
	return err
}

// CreateSAKey Creates a new GCP SA key
func (m *Yale) CreateSAKey(project string, saName string) (*SaKey, error) {
	logs.Info.Printf("Creating new SA key for %s", saName)
	// Expected naming convention for GCP i.am API
	name := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, saName)

	ctx := context.Background()
	rb := &iam.CreateServiceAccountKeyRequest{
		KeyAlgorithm:   KEY_ALGORITHM,
		PrivateKeyType: KEY_FORMAT,
	}
	newKey, err := m.gcp.Projects.ServiceAccounts.Keys.Create(name, rb).Context(ctx).Do()
	if err != nil {
		logs.Warn.Printf(" %v\n", err)
		return nil, err
	}
	return &SaKey{
		newKey.PrivateKeyData,
		newKey.Name,
		name,
		newKey.ValidAfterTime,
		newKey.Disabled,
	}, err

}

// GetSecret Returns a secret
func (m *Yale) GetSecret(Secret apiv1b1.Secret, namespace string) (*corev1.Secret, error) {
	return m.k8s.CoreV1().Secrets(namespace).Get(context.TODO(), Secret.Name, metav1.GetOptions{})
}

func (m *Yale) UpdateSecret(K8Secret *corev1.Secret) error {
	_, err := m.k8s.CoreV1().Secrets(K8Secret.Namespace).Update(context.TODO(), K8Secret, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	logs.Info.Printf("%s secret has been updated:", K8Secret.Name)
	return nil
}
