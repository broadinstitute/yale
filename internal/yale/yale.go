package yale

import (
	"context"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/config"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"google.golang.org/api/iam/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"log"
)

type Yale struct {
	config *config.Config       // Yale config
	gcp    *iam.Service    // GCP Compute API client
	k8s    kubernetes.Interface // K8s API client
}

type sakeyInfo struct {
	privateID string
}

// NewYale /* Construct a new Yale Manager */
func NewYale(cfg *config.Config, clients *client.Clients) (*Yale, error) {
	k8s := clients.GetK8s()
	gcp := clients.GetGCP()

	return &Yale{cfg, gcp, k8s}, nil
}

func (m *Yale) Run(){
	for _, secret := range m.config.SecretData{
		privateID := m.CreateSAKey(secret.GcpSaName)
		m.CreateSecret(secret.SecretName, secret.SecretDataKey, secret.Namespace, privateID)
	}
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
	logs.Info.Printf("Creating new SA key for ...", GcpSaName)
	ctx := context.Background()
	rb := &iam.CreateServiceAccountKeyRequest{KeyAlgorithm: "KEY_ALG_RSA_1024",
		PrivateKeyType: "TYPE_GOOGLE_CREDENTIALS_FILE"}
	newSAKey, err := m.gcp.Projects.ServiceAccounts.Keys.Create(GcpSaName, rb).Context(ctx).Do()
	if err != nil {
		log.Fatal(err)
	}
	return newSAKey.PrivateKeyData
}
