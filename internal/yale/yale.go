package yale

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/logs"
	v1crd "github.com/broadinstitute/yale/internal/yale/v1"
	"google.golang.org/api/iam/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"time"
)

type Yale struct {      // Yale config
	gcp    *iam.Service    // GCP Compute API client
	k8s    kubernetes.Interface // K8s API client
	crd	   restclient.Interface
}

type saKeyData struct {
	PrivateKey string `json:"private_key"`
}

type SaKey struct{
	privateKeyData string
	serviceAccountKeyName string
	serviceAccountName string
	validAfterTime string
}
// NewYale /* Construct a new Yale Manager */
func NewYale(clients *client.Clients) (*Yale, error) {
	k8s := clients.GetK8s()
	gcp := clients.GetGCP()
	crd := clients.GetCRDs()

	return &Yale{ gcp, k8s, crd }, nil
}

func (m *Yale) GenerateKeys(){
	// Get all GCPSaKey resources
	result, err := m.getGCPSaKeyList()
	if err != nil {
		panic(err)
	}

	for _, gcpsakey := range result.Items{

		secret, err := m.getSecret(gcpsakey.Spec)
		if err != nil {
			// Secret does not exist for GCPSaKey Crd
			if !secretExists(err, gcpsakey.Spec.SecretName){
				logs.Info.Printf("Secret for %s does not exist. Creating secret", gcpsakey.Name)
				// Create sa key
				newGCPSaKey := m.CreateSAKey(gcpsakey.Spec)
				m.CreateSecret(gcpsakey.Spec, *newGCPSaKey)
				logs.Info.Printf("Secret for %s has been created.", gcpsakey.Spec.SecretName)
			}else{
				logs.Error.Fatal(err)
			}
			continue
		}
		// A secret exists for the GCPSaKey Crd
		if m.isExpired(secret, gcpsakey.Spec){
			//Create sa key
			newGCPSaKey := m.CreateSAKey(gcpsakey.Spec)
			// Update secret with new SA key
			m.updateSecret(secret, gcpsakey.Spec,  *newGCPSaKey)
			logs.Info.Printf("%s secret has been updated:", gcpsakey.Spec.SecretName)
		}
	}
}

func (m *Yale) getGCPSaKeyList()(result v1crd.GCPSaKeyList, err error){
	result = v1crd.GCPSaKeyList{}

	// Get all GCPSaKey resources
	err = m.crd.Get().Resource("gcpsakeys").Do(context.TODO()).Into(&result)
	if err != nil {
		logs.Error.Fatal(err)
	}
	return
}

// Checks if secret exists
func secretExists(err error, secretName string) bool{
	secretNotFoundErrorMsg := fmt.Sprintf("secrets \"%s\" not found", secretName)
	return err.Error() != secretNotFoundErrorMsg
}

// CreateAnnotations Creates basic annotations based on Sa key
func CreateAnnotations(GcpSakey SaKey)map[string]string{
	var annotations = make(map[string]string)
	annotations["serviceAccountKeyName"] = GcpSakey.serviceAccountKeyName
	annotations["serviceAccountName"] = GcpSakey.serviceAccountName
	annotations["validAfterDate"] = GcpSakey.validAfterTime
	return annotations
}

func ( m *Yale ) CreateSecret(GCPSaKeySpec v1crd.GCPSaKeySpec, GcpSakey SaKey){
	logs.Info.Printf("Creating secret %s ...", GCPSaKeySpec.SecretName)
	saKey, err :=  base64.StdEncoding.DecodeString(GcpSakey.privateKeyData)

	if err != nil {
		logs.Error.Fatal(err)
	}
	saData := saKeyData{}
	err = json.Unmarshal([]byte(saKey), &saData)
	if err != nil {
		logs.Error.Fatal(err)
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: GCPSaKeySpec.Namespace,
			Name:      GCPSaKeySpec.SecretName,
			Labels: map[string]string{
				"app.kubernetes.io/instance": "yale",
			},
			Annotations: CreateAnnotations(GcpSakey),
		},
		StringData: map[string]string{
			GCPSaKeySpec.SecretDataKey : string(saKey),
			"service-account.pem":  saData.PrivateKey,
		},
		Type: v1.SecretTypeOpaque,
	}
	_, err = m.k8s.CoreV1().Secrets(secret.Namespace).Create(context.TODO(),secret, metav1.CreateOptions{})

	if err != nil {
		logs.Error.Fatal(err)
	}
}

func(m *Yale) updateSecret(K8Secret *v1.Secret, GCPSaKeySpec v1crd.GCPSaKeySpec, Key SaKey ){
	// Create annotations for new secret
	newAnnotations := CreateAnnotations(Key)

	// Add expired service account name to new secret for tracking
	oldAnnotations := K8Secret.GetAnnotations()
	oldSaKeyName := oldAnnotations["serviceAccountKeyName"]
	newAnnotations["oldServiceAccountKeyName"] = oldSaKeyName
	K8Secret.Annotations = newAnnotations


	K8Secret.Data[GCPSaKeySpec.SecretDataKey] = []byte(Key.privateKeyData)
	_, err := m.k8s.CoreV1().Secrets(GCPSaKeySpec.Namespace).Update(context.TODO(), K8Secret, metav1.UpdateOptions{})
	if err != nil {
		logs.Error.Fatal(oldAnnotations)
	}
}

func (m *Yale)CreateSAKey(SaKeySpec v1crd.GCPSaKeySpec) *SaKey {
	logs.Info.Printf("Creating new SA key for %s", SaKeySpec.GcpSaName)
	// Expected naming convention for GCP i.am API
	name := fmt.Sprintf("projects/%s/serviceAccounts/%s", SaKeySpec.GoogleProject, SaKeySpec.GcpSaName)

	ctx := context.Background()
	rb := &iam.CreateServiceAccountKeyRequest{KeyAlgorithm: "KEY_ALG_RSA_1024",
		PrivateKeyType: "TYPE_GOOGLE_CREDENTIALS_FILE"}
	newKey, err := m.gcp.Projects.ServiceAccounts.Keys.Create(name, rb).Context(ctx).Do()
	if err != nil {
		logs.Error.Fatal(err)
	}

	return &SaKey{
		newKey.PrivateKeyData,
		newKey.Name,
		name,
		newKey.ValidAfterTime,
	}
}

// Checks if key is expired
func (m *Yale) isExpired(K8Secret *v1.Secret, GCPSaKeySpec v1crd.GCPSaKeySpec)bool {
	ctx := context.Background()
	annotations := K8Secret.GetAnnotations()
	saKeyName := annotations["serviceAccountKeyName"]
	logs.Info.Printf("Checking if key, %s, is expiring", saKeyName )

	resp, err := m.gcp.Projects.ServiceAccounts.Keys.Get(saKeyName).Context(ctx).Do()
	if err != nil {
		logs.Error.Fatal(err)
	}
	// Parse date from string to date
	dateAuthorized, err := time.Parse("2006-01-02T15:04:05Z0700",resp.ValidAfterTime)
	if err != nil {
		logs.Error.Fatal(err)
	}
	// Date sa key expected to expire
	expireDate := dateAuthorized.AddDate(0, 0, GCPSaKeySpec.OlderThanDays)
	if time.Now().After(expireDate) {
		logs.Info.Printf("%s had expired", saKeyName)
		return true
	}
	logs.Info.Printf("%s has not expired", saKeyName)
	return false
}

func (m *Yale) getSecret(GCPSaKeySpec v1crd.GCPSaKeySpec )(*v1.Secret, error){
	return m.k8s.CoreV1().Secrets(GCPSaKeySpec.Namespace).Get(context.TODO(), GCPSaKeySpec.SecretName, metav1.GetOptions{})
}