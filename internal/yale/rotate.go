package yale

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/client"
	apiv1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1"
	clientv1 "github.com/broadinstitute/yale/internal/yale/crd/clientset/v1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/policyanalyzer/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"time"
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
	googleProject         string
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
	} else {
		// Iterate through GSK resources
		for _, gcpsakey := range result.Items {

			secret, err := m.GetSecret(gcpsakey.Spec, metav1.GetOptions{})
			if err != nil {
				// Secret has never been created for GSK
				if !secretExists(err, gcpsakey.Spec.SecretName) {
					logs.Info.Printf("Secret %s for %s does not exist. Creating secret", gcpsakey.Spec.SecretName, gcpsakey.Name)
					newGCPSaKey, err := m.CreateSAKey(gcpsakey.Spec) // Create SA key
					if newGCPSaKey != nil {
						err := m.CreateSecret(gcpsakey.Spec, *newGCPSaKey)
						if err != nil {
							return err
						}
						logs.Info.Printf("Secret for %s has been created.", gcpsakey.Spec.SecretName)
					} else {
						return err
					}
				}
				return err
			} else {
				keyIsExpired, err := m.isExpired(secret, gcpsakey.Spec) // Check if key has expired
				if err != nil {
					return err
				}
				if keyIsExpired {
					newGCPSaKey, err := m.CreateSAKey(gcpsakey.Spec) // Create new SA key to replace the old one
					if err != nil {
						return err
					}
					err = m.UpdateSecretWithNewKey(secret, gcpsakey.Spec, *newGCPSaKey) // Update secret with new SA key
				}
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// GetGCPSaKeyList Returns list of GSKs
func (m *Yale) GetGCPSaKeyList() (result *apiv1.GCPSaKeyList, err error) {
	return m.crd.GcpSaKeys().List(context.Background(), metav1.ListOptions{})
}

// secretExists Checks if secret exists with given name
func secretExists(err error, secretName string) bool {
	secretNotFoundErrorMsg := fmt.Sprintf("secrets \"%s\" not found", secretName)
	return err.Error() != secretNotFoundErrorMsg
}

// Creates basic annotations for Secret
func createAnnotations(GcpSakey SaKey) map[string]string {
	var annotations = make(map[string]string)
	annotations["googleProject"] = GcpSakey.googleProject
	annotations["serviceAccountKeyName"] = GcpSakey.serviceAccountKeyName
	annotations["serviceAccountName"] = GcpSakey.serviceAccountName
	annotations["validAfterDate"] = GcpSakey.validAfterTime
	annotations["reloader.stakater.com/match"] = "true"
	return annotations
}

// CreateSecret Creates a secret for a new GSK resource
func (m *Yale) CreateSecret(GCPSaKeySpec apiv1.GCPSaKeySpec, GcpSakey SaKey) error {
	logs.Info.Printf("Creating secret %s ...", GCPSaKeySpec.SecretName)
	saKey, err := base64.StdEncoding.DecodeString(GcpSakey.privateKeyData)

	if err != nil {
		return err

	} else {
		saData := saKeyData{}
		err = json.Unmarshal(saKey, &saData)
		if err != nil {
			return err
		} else {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   GCPSaKeySpec.Namespace,
					Name:        GCPSaKeySpec.SecretName,
					Labels:      GCPSaKeySpec.Labels,
					Annotations: createAnnotations(GcpSakey),
				},
				StringData: map[string]string{
					GCPSaKeySpec.PrivateKeyDataFieldName: string(saKey),
					GCPSaKeySpec.PemDataFieldName:        saData.PrivateKey,
				},
				Type: corev1.SecretTypeOpaque,
			}
			_, err = m.k8s.CoreV1().Secrets(secret.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})

			if err != nil {
				return err
			}
		}
	}
	return nil
}

// UpdateSecretWithNewKey Updates pem data and private key data fields in Secret with new key
func (m *Yale) UpdateSecretWithNewKey(K8Secret *corev1.Secret, GCPSaKeySpec apiv1.GCPSaKeySpec, Key SaKey) error {

	// Create annotations for new secret
	newAnnotations := createAnnotations(Key)

	// Add expired service account name to new secret for tracking
	oldAnnotations := K8Secret.GetAnnotations()
	oldSaKeyName := oldAnnotations["serviceAccountKeyName"]
	newAnnotations["oldServiceAccountKeyName"] = oldSaKeyName

	K8Secret.Annotations = newAnnotations // Set the secret's annotations

	saKey, err := base64.StdEncoding.DecodeString(Key.privateKeyData)

	if err != nil {
		return err

	} else {
		saData := saKeyData{}
		err = json.Unmarshal(saKey, &saData)
		if err != nil {
			return err
		} else {

			K8Secret.Data[GCPSaKeySpec.PrivateKeyDataFieldName] = []byte(Key.privateKeyData)
			K8Secret.Data[GCPSaKeySpec.PemDataFieldName] = []byte(saData.PrivateKey)
			err := m.UpdateSecret(GCPSaKeySpec, K8Secret)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// CreateSAKey Creates a new GCP SA key
func (m *Yale) CreateSAKey(SaKeySpec apiv1.GCPSaKeySpec) (*SaKey, error) {
	logs.Info.Printf("Creating new SA key for %s", SaKeySpec.GcpSaName)
	// Expected naming convention for GCP i.am API
	name := fmt.Sprintf("projects/%s/serviceAccounts/%s", SaKeySpec.GoogleProject, SaKeySpec.GcpSaName)

	ctx := context.Background()
	rb := &iam.CreateServiceAccountKeyRequest{
		KeyAlgorithm:   KEY_ALGORITHM,
		PrivateKeyType: KEY_FORMAT,
	}
	newKey, err := m.gcp.Projects.ServiceAccounts.Keys.Create(name, rb).Context(ctx).Do()
	if err != nil {
		logs.Warn.Printf(" %v\n", err)
		return nil, err
	} else {
		return &SaKey{
			SaKeySpec.GoogleProject,
			newKey.PrivateKeyData,
			newKey.Name,
			name,
			newKey.ValidAfterTime,
			newKey.Disabled,
		}, err
	}
}

// Checks if key is expired
func (m *Yale) isExpired(K8Secret *corev1.Secret, GCPSaKeySpec apiv1.GCPSaKeySpec) (bool, error) {

	ctx := context.Background()
	annotations := K8Secret.GetAnnotations()
	saKeyName := annotations["serviceAccountKeyName"]
	logs.Info.Printf("Checking if key, %s, is expiring", saKeyName)

	resp, err := m.gcp.Projects.ServiceAccounts.Keys.Get(saKeyName).Context(ctx).Do()
	if err != nil {
		return false, err
	}
	// Parse date from string to date
	dateAuthorized, err := time.Parse("2006-01-02T15:04:05Z0700", resp.ValidAfterTime)
	if err != nil {
		return false, err
	}
	// Date sa key expected to expire
	expireDate := dateAuthorized.AddDate(0, 0, GCPSaKeySpec.OlderThanDays)
	if time.Now().After(expireDate) {
		logs.Info.Printf("%s had expired", saKeyName)
		return true, nil
	}
	logs.Info.Printf("%s has not expired", saKeyName)
	return false, nil
}

// GetSecret Returns a secret
func (m *Yale) GetSecret(GCPSaKeySpec apiv1.GCPSaKeySpec, getOptions metav1.GetOptions) (*corev1.Secret, error) {
	return m.k8s.CoreV1().Secrets(GCPSaKeySpec.Namespace).Get(context.TODO(), GCPSaKeySpec.SecretName, getOptions)
}

func (m *Yale) UpdateSecret(GCPSaKeySpec apiv1.GCPSaKeySpec, K8Secret *corev1.Secret) error {
	_, err := m.k8s.CoreV1().Secrets(GCPSaKeySpec.Namespace).Update(context.TODO(), K8Secret, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	logs.Info.Printf("%s secret has been updated:", GCPSaKeySpec.SecretName)
	return nil
}
