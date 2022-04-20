package client

// Taken from disk manager
// https://github.com/broadinstitute/disk-manager/
import (
	"fmt"
	v1beta1crd "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	v1beta1 "github.com/broadinstitute/yale/internal/yale/crd/clientset/v1beta1"
	"golang.org/x/net/context"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/policyanalyzer/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Build will return a k8s client using local kubectl
// config

// Clients struct containing the GCP and k8s clients used in this tool
type Clients struct {
	gcpIAM *iam.Service
	gcpPA  *policyanalyzer.Service
	k8s    kubernetes.Interface
	crd    v1beta1.YaleCRDInterface
}

func NewClients(gcpIAM *iam.Service, gcpPA *policyanalyzer.Service, k8s kubernetes.Interface, crd v1beta1.YaleCRDInterface) *Clients {
	return &Clients{
		// Service for Google IAM
		gcpIAM: gcpIAM,
		// Service for Google policy analyzer
		gcpPA: gcpPA,
		k8s:   k8s,
		crd:   crd,
	}
}

// GetGCP will return a handle to the gcp IAM client generated by the builder
func (c *Clients) GetGCP() *iam.Service {
	return c.gcpIAM
}

// GetK8s will return  a handle to the kubernetes client generated by the builder
func (c *Clients) GetK8s() kubernetes.Interface {
	return c.k8s
}

// GetGCPPA will return  a handle to the policy analyzer client generated by the builder
func (c *Clients) GetGCPPA() *policyanalyzer.Service {
	return c.gcpPA
}

// GetCRDs will return  a handle to the crd client generated by the builder
func (c *Clients) GetCRDs() v1.YaleCRDInterface {
	return c.crd
}

// Build creates the GCP and k8s clients used by this tool
// and returns both packaged in a single struct
func Build(local bool, kubeconfig string) (*Clients, error) {
	conf, err := buildKubeConfig(local, kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("error building kube client: %v", err)
	}
	k8s, err := buildKubeClient(conf)
	if err != nil {
		return nil, fmt.Errorf("error building kube client: %v", err)
	}

	gcpIam, err := buildGCPIAMClient()
	if err != nil {
		return nil, fmt.Errorf("error building GCP IAM client: %v", err)
	}
	gcpPA, err := buildGCPPolicyAnalyzerClient()
	if err != nil {
		return nil, fmt.Errorf("error building GCP Policy Analyzer client: %v", err)
	}
	crd, err := buildCrdClient(conf)
	if err != nil {
		return nil, fmt.Errorf("error building GCP client: %v", err)
	}
	return NewClients(gcpIam, gcpPA, k8s, crd), nil
}

func buildKubeConfig(local bool, kubeconfig string) (*restclient.Config, error) {
	if local {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("error building local k8s config: %v", err)
		}
		return config, nil
	}
	config, err := restclient.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("error building in cluster k8s config: %v", err)
	}
	return config, nil
}

func buildKubeClient(config *restclient.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(config)
}
func buildGCPIAMClient() (*iam.Service, error) {
	ctx := context.Background()
	c, err := iam.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating iam api client: %v", err)
	}
	return c, nil
}
func buildGCPPolicyAnalyzerClient() (*policyanalyzer.Service, error) {
	ctx := context.Background()
	c, err := policyanalyzer.NewService(ctx)

	if err != nil {
		return nil, fmt.Errorf("error creating iam api client: %v", err)
	}
	return c, nil
}

func buildCrdClient(kubeconfig *restclient.Config) (*v1beta1.YaleCRDClient, error) {
	if err := v1beta1crd.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}

	return v1beta1.NewForConfig(kubeconfig)
}
