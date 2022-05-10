package yale

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/client"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	v1beta1client "github.com/broadinstitute/yale/internal/yale/crd/clientset/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/policyanalyzer/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"regexp"
)

// KEY_ALGORITHM what key algorithm to use when creating new Google SA keys
const KEY_ALGORITHM string = "KEY_ALG_RSA_2048"

// KEY_FORMAT format to use when creating new Google SA keys
const KEY_FORMAT string = "TYPE_GOOGLE_CREDENTIALS_FILE"

// Returns service account name
// Ex: agora-perf-service-account@broad-dsde-perf.iam.gserviceaccount.com returns agora-perf-service-account
var r, _ = regexp.Compile("^[^@]*")

type Yale struct { // Yale config
	gcp   *iam.Service                   // GCP IAM API client
	gcpPA *policyanalyzer.Service        // GCP Policy API client
	k8s   kubernetes.Interface           // K8s API client
	crd   v1beta1client.YaleCRDInterface // K8s CRD API client
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

func (m *Yale) rotateKey(gsk apiv1b1.GCPSaKey) error {
	exists, err := m.secretExists(gsk.Spec.Secret, gsk.Namespace)
	if err != nil {
		return err
	}
	if !exists {

		return m.CreateSecret(gsk)
	} else {
		return m.UpdateKey(gsk.Spec, gsk.Namespace)
	}
}

func (m *Yale) secretExists(secret apiv1b1.Secret, namespace string) (bool, error) {
	_, err := m.GetSecret(secret, namespace)
	if err == nil {
		return true, nil
	}
	if errors.IsNotFound(err) {
		logs.Info.Printf("%s does not exist", secret.Name)
		return false, nil
	}
	return false, err
}

// GetGCPSaKeyList Returns list of GSKs
func (m *Yale) GetGCPSaKeyList() (result *apiv1b1.GCPSaKeyList, err error) {
	return m.crd.GcpSaKeys().List(context.Background(), metav1.ListOptions{})
}

// createAnnotations Creates basic annotations for Secret
func createAnnotations(key SaKey) map[string]string {
	return map[string]string{
		"serviceAccountKeyName":       key.serviceAccountKeyName,
		"serviceAccountName":          key.serviceAccountName,
		"validAfterDate":              key.validAfterTime,
		"reloader.stakater.com/match": "true",
	}
}

// CreateSecret Creates a secret for a new GSK resource
func (m *Yale) CreateSecret(gsk apiv1b1.GCPSaKey) error {
	logs.Info.Printf("Attempting to create secret %s ...", gsk.Spec.Secret.Name)
	saKey, err := m.CreateSAKey(gsk.Spec.GoogleServiceAccount.Project, gsk.Spec.GoogleServiceAccount.Name)

	if err != nil {
		return err
	}
	jsonKey, err := base64.StdEncoding.DecodeString(saKey.privateKeyData)
	if err != nil {
		logs.Error.Printf("Cannot decode %s: %v ", r.FindString(gsk.Spec.GoogleServiceAccount.Name), err)
		return err
	}
	saData := saKeyData{}
	err = json.Unmarshal(jsonKey, &saData)
	if err != nil {
		logs.Error.Printf("Cannot unmarshal %s: %v ", r.FindString(gsk.Spec.GoogleServiceAccount.Name), err)
		return err
	}

	// Create ownership reference
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents
	var ownerRef = []metav1.OwnerReference{
		{
			APIVersion: gsk.APIVersion,
			Kind:       gsk.Kind,
			Name:       gsk.Name,
			UID:        gsk.UID,
		},
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       gsk.Namespace,
			Name:            gsk.Spec.Secret.Name,
			Labels:          gsk.Labels,
			Annotations:     createAnnotations(*saKey),
			OwnerReferences: ownerRef,
		},
		StringData: map[string]string{
			gsk.Spec.Secret.JsonKeyName: string(jsonKey),
			gsk.Spec.Secret.PemKeyName:  saData.PrivateKey,
		},
		Type: corev1.SecretTypeOpaque,
	}
	_, err = m.k8s.CoreV1().Secrets(gsk.Namespace).Create(context.TODO(), newSecret, metav1.CreateOptions{})

	if err != nil {
		return err
	}
	logs.Info.Printf("Successfully created secret %s for %s", gsk.Spec.Secret.Name, r.FindString(gsk.Spec.GoogleServiceAccount.Name))
	return nil
}

//UpdateKey Updates pem data and private key data fields in Secret with new key
func (m *Yale) UpdateKey(gskSpec apiv1b1.GCPSaKeySpec, namespace string) error {
	K8Secret, err := m.GetSecret(gskSpec.Secret, namespace)
	if err != nil {
		return err
	}
	// Annotations are not queryable
	originalAnnotations := K8Secret.GetAnnotations()
	keyIsExpired, err := IsExpired(originalAnnotations["validAfterDate"], gskSpec.KeyRotation.RotateAfter)
	if err != nil {
		return err
	}
	if !keyIsExpired {
		logs.Info.Printf("It is not time to rotate %v. ", r.FindString(gskSpec.GoogleServiceAccount.Name))
		return nil
	}
	logs.Info.Printf("Time to rotate %v ", r.FindString(gskSpec.GoogleServiceAccount.Name))
	Key, err := m.CreateSAKey(gskSpec.GoogleServiceAccount.Project, gskSpec.GoogleServiceAccount.Name)
	if err != nil {
		return err
	}
	// Create annotations for new key
	newAnnotations := createAnnotations(*Key)
	// Record old key's name for tracking
	newAnnotations["oldServiceAccountKeyName"] = originalAnnotations["serviceAccountKeyName"]
	// Update valid after date to new key validAfterTime
	newAnnotations["validAfterDate"] = Key.validAfterTime
	K8Secret.ObjectMeta.SetAnnotations(newAnnotations)

	saKey, err := base64.StdEncoding.DecodeString(Key.privateKeyData)
	if err != nil {
		logs.Error.Printf("Cannot decode %s: %v ", r.FindString(gskSpec.GoogleServiceAccount.Name), err)
		return err
	}
	saData := saKeyData{}
	err = json.Unmarshal(saKey, &saData)
	if err != nil {
		logs.Error.Printf("Cannot unmarshal %s: %v ", r.FindString(gskSpec.GoogleServiceAccount.Name), err)
		return err
	}
	K8Secret.Data[gskSpec.Secret.JsonKeyName] = saKey
	K8Secret.Data[gskSpec.Secret.PemKeyName] = []byte(saData.PrivateKey)
	return m.UpdateSecret(K8Secret)
}

// CreateSAKey Creates a new GCP SA key
func (m *Yale) CreateSAKey(project string, saName string) (*SaKey, error) {
	logs.Info.Printf("Starting to create new key for %s", r.FindString(saName))
	// Expected naming convention for GCP i.am API
	name := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, saName)

	ctx := context.Background()
	rb := &iam.CreateServiceAccountKeyRequest{
		KeyAlgorithm:   KEY_ALGORITHM,
		PrivateKeyType: KEY_FORMAT,
	}
	newKey, err := m.gcp.Projects.ServiceAccounts.Keys.Create(name, rb).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	logs.Info.Printf("Created key for %s", r.FindString(saName))
	return &SaKey{
		newKey.PrivateKeyData,
		newKey.Name,
		name,
		newKey.ValidAfterTime,
		newKey.Disabled,
	}, err

}

// GetSecret Returns a secret
func (m *Yale) GetSecret(secret apiv1b1.Secret, namespace string) (*corev1.Secret, error) {
	return m.k8s.CoreV1().Secrets(namespace).Get(context.TODO(), secret.Name, metav1.GetOptions{})
}

func (m *Yale) UpdateSecret(k8Secret *corev1.Secret) error {
	_, err := m.k8s.CoreV1().Secrets(k8Secret.Namespace).Update(context.TODO(), k8Secret, metav1.UpdateOptions{})
	if err != nil {
		logs.Error.Printf("Error updating secret %s: %v\n", k8Secret.Name, err)
		return err
	}
	logs.Info.Printf("%s secret has been updated", k8Secret.Name)
	return nil
}
