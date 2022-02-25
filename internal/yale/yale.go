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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"time"
)

// keyAlgorithm what key algorithm to use when creating new Google SA keys
const keyAlgorithm = "KEY_ALG_RSA_2048"

// keyFormat format to use when creating new Google SA keys
const keyFormat = "TYPE_GOOGLE_CREDENTIALS_FILE"

type Yale struct { // Yale config
	gcp *iam.Service              // GCP Compute API client
	k8s kubernetes.Interface      // K8s API client
	crd clientv1.YaleCRDInterface // K8s CRD API client
}

type saKeyData struct {
	PrivateKey string `json:"private_key"`
}

type SaKey struct {
	privateKeyData        string
	serviceAccountKeyName string
	serviceAccountName    string
	validAfterTime        string
}

// NewYale /* Construct a new Yale Manager */
func NewYale(clients *client.Clients) (*Yale, error) {
	k8s := clients.GetK8s()
	gcp := clients.GetGCP()
	crd := clients.GetCRDs()

	return &Yale{gcp, k8s, crd}, nil
}

func (m *Yale) GenerateKeys() {
	// Get all GCPSaKey resources
	result, err := m.getGCPSaKeyList()
	if err != nil {
		logs.Warn.Printf(" %v\n", err)
	} else {

		for _, gcpsakey := range result.Items {

			secret, err := m.getSecret(gcpsakey.Spec)
			if err != nil {
				// Secret does not exist for GCPSaKey Crd
				if !secretExists(err, gcpsakey.Spec.SecretName) {
					logs.Info.Printf("Secret %s for %s does not exist. Creating secret", gcpsakey.Spec.SecretName, gcpsakey.Name)
					// Create sa key
					newGCPSaKey := m.CreateSAKey(gcpsakey.Spec)
					if newGCPSaKey != nil {
						m.CreateSecret(gcpsakey.Spec, *newGCPSaKey)
						logs.Info.Printf("Secret for %s has been created.", gcpsakey.Spec.SecretName)
					}
				}
				continue
			} else {
				keyIsExpired, err := m.isExpired(secret, gcpsakey.Spec)

				if keyIsExpired {
					//Create sa key
					newGCPSaKey := m.CreateSAKey(gcpsakey.Spec)
					// Update secret with new SA key
					m.updateSecret(secret, gcpsakey.Spec, *newGCPSaKey)
				}
				if err != nil {
					logs.Warn.Printf(" %v\n", err)
				}
			}
		}
	}
}

func (m *Yale) getGCPSaKeyList() (result *apiv1.GCPSaKeyList, err error) {
	return m.crd.GcpSaKeys().List(context.Background(), metav1.ListOptions{})
}

// Checks if secret exists
func secretExists(err error, secretName string) bool {
	secretNotFoundErrorMsg := fmt.Sprintf("secrets \"%s\" not found", secretName)
	return err.Error() != secretNotFoundErrorMsg
}

// CreateAnnotations Creates basic annotations based on Sa key
func CreateAnnotations(GcpSakey SaKey) map[string]string {
	var annotations = make(map[string]string)
	annotations["serviceAccountKeyName"] = GcpSakey.serviceAccountKeyName
	annotations["serviceAccountName"] = GcpSakey.serviceAccountName
	annotations["validAfterDate"] = GcpSakey.validAfterTime
	return annotations
}

func (m *Yale) CreateSecret(GCPSaKeySpec apiv1.GCPSaKeySpec, GcpSakey SaKey) {
	logs.Info.Printf("Creating secret %s ...", GCPSaKeySpec.SecretName)
	saKey, err := base64.StdEncoding.DecodeString(GcpSakey.privateKeyData)

	if err != nil {
		logs.Warn.Printf(" %v\n", err)

	} else {
		saData := saKeyData{}
		err = json.Unmarshal([]byte(saKey), &saData)
		if err != nil {
			logs.Warn.Printf(" %v\n", err)
		} else {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   GCPSaKeySpec.Namespace,
					Name:        GCPSaKeySpec.SecretName,
					Labels:      GCPSaKeySpec.Labels,
					Annotations: CreateAnnotations(GcpSakey),
				},
				StringData: map[string]string{
					GCPSaKeySpec.SecretDataKey:    string(saKey),
					GCPSaKeySpec.PemDataFieldName: saData.PrivateKey,
				},
				Type: corev1.SecretTypeOpaque,
			}
			_, err = m.k8s.CoreV1().Secrets(secret.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})

			if err != nil {
				logs.Warn.Printf(" %v\n", err)
			}
		}
	}
}

func (m *Yale) updateSecret(K8Secret *corev1.Secret, GCPSaKeySpec apiv1.GCPSaKeySpec, Key SaKey) {

	// Create annotations for new secret
	newAnnotations := CreateAnnotations(Key)

	// Add expired service account name to new secret for tracking
	oldAnnotations := K8Secret.GetAnnotations()
	oldSaKeyName := oldAnnotations["serviceAccountKeyName"]
	newAnnotations["oldServiceAccountKeyName"] = oldSaKeyName
	K8Secret.Annotations = newAnnotations

	saKey, err := base64.StdEncoding.DecodeString(Key.privateKeyData)

	if err != nil {
		logs.Warn.Printf(" %v\n", err)

	} else {
		saData := saKeyData{}
		err = json.Unmarshal([]byte(saKey), &saData)
		if err != nil {
			logs.Warn.Printf(" %v\n", err)
		} else {

			K8Secret.Data[GCPSaKeySpec.SecretDataKey] = []byte(Key.privateKeyData)
			K8Secret.Data[GCPSaKeySpec.PemDataFieldName] = []byte(saData.PrivateKey)
			_, err := m.k8s.CoreV1().Secrets(GCPSaKeySpec.Namespace).Update(context.TODO(), K8Secret, metav1.UpdateOptions{})
			if err != nil {
				logs.Warn.Printf(" %v\n", oldAnnotations)
			}
			logs.Info.Printf("%s secret has been updated:", GCPSaKeySpec.SecretName)
		}
	}
}

func (m *Yale) CreateSAKey(SaKeySpec apiv1.GCPSaKeySpec) *SaKey {
	logs.Info.Printf("Creating new SA key for %s", SaKeySpec.GcpSaName)
	// Expected naming convention for GCP i.am API
	name := fmt.Sprintf("projects/%s/serviceAccounts/%s", SaKeySpec.GoogleProject, SaKeySpec.GcpSaName)

	ctx := context.Background()
	rb := &iam.CreateServiceAccountKeyRequest{
		KeyAlgorithm:   keyAlgorithm,
		PrivateKeyType: keyFormat,
	}
	newKey, err := m.gcp.Projects.ServiceAccounts.Keys.Create(name, rb).Context(ctx).Do()
	if err != nil {
		logs.Warn.Printf(" %v\n", err)
		return nil
	} else {
		return &SaKey{
			newKey.PrivateKeyData,
			newKey.Name,
			name,
			newKey.ValidAfterTime,
		}
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

func (m *Yale) getSecret(GCPSaKeySpec apiv1.GCPSaKeySpec) (*corev1.Secret, error) {
	return m.k8s.CoreV1().Secrets(GCPSaKeySpec.Namespace).Get(context.TODO(), GCPSaKeySpec.SecretName, metav1.GetOptions{})
}
