package yale

import (
	"context"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/logs"
	v1crd "github.com/broadinstitute/yale/internal/yale/v1"
	"google.golang.org/api/iam/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"log"
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
	//privateID := m.CreateSAKey("hello")
	result := v1crd.GCPSaKeyList{}
	err := m.crd.Get().Resource("gcpsakey").Do(context.TODO()).Into(&result)
	if err != nil {
		panic(err)
	}
	for _, secretCRD := range result.Items{
		fmt.Printf("SecretDefinition: %s\n", secretCRD.Name)
		fmt.Printf("Namespace: %s\n", secretCRD.Namespace)
		fmt.Printf("Mappings:\n")
	}
		/*m.CreateSecret(secret.SecretName, secret.SecretDataKey, secret.Namespace, privateID)*/
}

func ( m *Yale) CreateSecret(SecretName string, SecretKey string, namespace string, privateID string){
	logs.Info.Printf("Creating secret for %s clients...", SecretName)
	saKey :=  []byte(privateID)
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      SecretName,
		},
		StringData: map[string]string{
			"my-key": string(saKey),
		},
		Type: v1.SecretTypeOpaque,
	}
	secrets, err := m.k8s.CoreV1().Secrets(secret.Namespace).Create(context.TODO(),secret, metav1.CreateOptions{})

	if err != nil {
		log.Fatal(secrets)
	}
}

/*fun updawteKey(){
//secrets, err := m.k8s.CoreV1().Secrets("default").Get(context.TODO(),SecretName, metav1.GetOptions{})
//secrets.Data[SecretKey]=  []byte(privateID)
//m.k8s.CoreV1().Secrets("default").Update(context.TODO(), secrets, metav1.UpdateOptions{})
}*/
func (m *Yale)CreateSAKey(GcpSaName string) string {
	logs.Info.Printf("Creating new SA key for %s", GcpSaName)
	ctx := context.Background()
	rb := &iam.CreateServiceAccountKeyRequest{KeyAlgorithm: "KEY_ALG_RSA_1024",
		PrivateKeyType: "TYPE_GOOGLE_CREDENTIALS_FILE"}
	newSAKey, err := m.gcp.Projects.ServiceAccounts.Keys.Create(GcpSaName, rb).Context(ctx).Do()
	if err != nil {
		log.Fatal(err)
	}
	return newSAKey.PrivateKeyData
}
