package yale

import (
	"context"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/client"
	logs "github.com/broadinstitute/yale/internal/yale/logs"
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
	crd restclient.Interface
}

type GCPSaKeyDefinition struct{
	privateKeyData string
	serviceAccountKeyName string
	serviceAccountName string
}
// NewYale /* Construct a new Yale Manager */
func NewYale(clients *client.Clients) (*Yale, error) {
	k8s := clients.GetK8s()
	gcp := clients.GetGCP()
	crd := clients.GetCRDs()

	return &Yale{ gcp, k8s, crd, }, nil
}

func (m *Yale) Run(){
	result := v1crd.GCPSaKeyList{}
	err := m.crd.Get().Resource("gcpsakeys").Do(context.TODO()).Into(&result)
	if err != nil {
		panic(err)
	}

	for _, gcpsakey := range result.Items{

		secret, err := m.getSecret(gcpsakey.Spec)
		if err != nil {
			secretNotFoundErrorMsg := fmt.Sprintf("secrets \"%s\" not found", gcpsakey.Spec.SecretName)

			// Secret does not exist for GCPSaKey Crd
			if err.Error() == secretNotFoundErrorMsg{
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
		// Check if the sa key is expiring
		if m.isExpiring(*secret, gcpsakey.Spec){
			//Create sa key
			newGCPSaKey := m.CreateSAKey(gcpsakey.Spec)
			// Update secret with new SA key
			m.updateSecret(secret, gcpsakey.Spec,  *newGCPSaKey)
			logs.Info.Printf("%s secret has been updated:", gcpsakey.Spec.SecretName)
		}
	}
}

func CreateAnnotations(GcpSakey GCPSaKeyDefinition)map[string]string{
	annotations := make(map[string]string)
	annotations["serviceAccountKeyName"] = GcpSakey.serviceAccountKeyName
	annotations["serviceAccountName"] = GcpSakey.serviceAccountName
	return annotations
}

func ( m *Yale, ) CreateSecret(SaKeySpec v1crd.GCPSaKeySpec, GcpSakey GCPSaKeyDefinition){
	logs.Info.Printf("Creating secret %s ...", SaKeySpec.SecretName)
	saKey :=  []byte(GcpSakey.privateKeyData)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: SaKeySpec.Namespace,
			Name:      SaKeySpec.SecretName,
			Annotations: CreateAnnotations(GcpSakey),
		},
		StringData: map[string]string{
			SaKeySpec.SecretDataKey : string(saKey),
		},
		Type: v1.SecretTypeOpaque,
	}
	secrets, err := m.k8s.CoreV1().Secrets(secret.Namespace).Create(context.TODO(),secret, metav1.CreateOptions{})

	if err != nil {
		logs.Error.Fatal(secrets)
	}
}

func(m *Yale) updateSecret(K8Secret *v1.Secret, SaKeySpec v1crd.GCPSaKeySpec, NewSakey GCPSaKeyDefinition, ){
	K8Secret.Annotations = CreateAnnotations(NewSakey)

	K8Secret.Data[SaKeySpec.SecretDataKey] = []byte(NewSakey.privateKeyData)
	m.k8s.CoreV1().Secrets("default").Update(context.TODO(), K8Secret, metav1.UpdateOptions{})
}

func (m *Yale)CreateSAKey(SaKeySpec v1crd.GCPSaKeySpec) *GCPSaKeyDefinition {
	// Expected naming convention for GCP i.am API
	name := fmt.Sprintf("projects/%s/serviceAccounts/%s", SaKeySpec.GoogleProject, SaKeySpec.GcpSaName)
	logs.Info.Printf("Creating new SA key for %s", SaKeySpec.GcpSaName)
	ctx := context.Background()
	rb := &iam.CreateServiceAccountKeyRequest{KeyAlgorithm: "KEY_ALG_RSA_1024",
		PrivateKeyType: "TYPE_GOOGLE_CREDENTIALS_FILE"}
	newSAKey, err := m.gcp.Projects.ServiceAccounts.Keys.Create(name, rb).Context(ctx).Do()
	if err != nil {
		logs.Error.Fatal(err)
	}
	return &GCPSaKeyDefinition{ privateKeyData: newSAKey.PrivateKeyData, serviceAccountKeyName: newSAKey.Name, serviceAccountName: name}
}

func (m *Yale) isExpiring(K8Secret v1.Secret, SaKeySpec v1crd.GCPSaKeySpec)bool {
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
	expireDate := dateAuthorized.AddDate(0, 0, SaKeySpec.OlderThanDays)
	if time.Now().After(expireDate) {
		logs.Info.Printf("%s is expiring", saKeyName)
		return true
	}
	return false
}

func (m *Yale) getSecret(SaKeySpec v1crd.GCPSaKeySpec )(*v1.Secret, error){
	return m.k8s.CoreV1().Secrets(SaKeySpec.Namespace).Get(context.TODO(), SaKeySpec.SecretName, metav1.GetOptions{})
}