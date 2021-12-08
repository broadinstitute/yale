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
		officialGcpSaName := fmt.Sprintf("projects/%s/serviceAccounts/%s", gcpsakey.Spec.GoogleProject, gcpsakey.Spec.GcpSaName)
		//Determine if there are any expiring keys
		if m.isExpiring(officialGcpSaName, gcpsakey.Spec.OlderThanDays){
			//Get private ID of newly created key
			privateID := m.CreateSAKey(officialGcpSaName)
			// Create new secret
			m.CreateSecret(gcpsakey.Spec, privateID)
		}
	}
}

func ( m *Yale) CreateSecret(SaKeySpec v1crd.GCPSaKeySpec, privateID string){
	logs.Info.Printf("Creating secret %s ...", SaKeySpec.SecretName)
	saKey :=  []byte(privateID)
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: SaKeySpec.Namespace,
			Name:      SaKeySpec.SecretName,
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

/*fun updawteKey(){
//secrets, err := m.k8s.CoreV1().Secrets("default").Get(context.TODO(),SecretName, metav1.GetOptions{})
//secrets.Data[SecretKey]=  []byte(privateID)
//m.k8s.CoreV1().Secrets("default").Update(context.TODO(), secrets, metav1.UpdateOptions{})
}*/

func (m *Yale)CreateSAKey(officialGcpSaName string) string {

	logs.Info.Printf("Creating new SA key for %s", officialGcpSaName)
	ctx := context.Background()
	rb := &iam.CreateServiceAccountKeyRequest{KeyAlgorithm: "KEY_ALG_RSA_1024",
		PrivateKeyType: "TYPE_GOOGLE_CREDENTIALS_FILE"}
	newSAKey, err := m.gcp.Projects.ServiceAccounts.Keys.Create(officialGcpSaName, rb).Context(ctx).Do()
	if err != nil {
		logs.Error.Fatal(err)
	}
	return newSAKey.PrivateKeyData
}

func (m *Yale) isExpiring(officialGcpSaName string,DaysAuthorized int)bool {
	ctx := context.Background()
	resp, err := m.gcp.Projects.ServiceAccounts.Keys.List(officialGcpSaName).KeyTypes("USER_MANAGED").Context(ctx).Do()
	saKeys := resp.Keys
	logs.Info.Printf("Checking if %s is expiring", saKeys)
	if err != nil {
		logs.Error.Fatal(err)
	}
	for _, sa := range saKeys {
		// Parse date from string to date
		dateAuthorized, err := time.Parse("2006-01-02T15:04:05Z0700",sa.ValidAfterTime)
		if err != nil {
			logs.Error.Fatal(err)
		}
		expireDate := dateAuthorized.AddDate(0, 0, DaysAuthorized)
		if time.Now().After(expireDate) {
			return true
		}
	}
	return false
}