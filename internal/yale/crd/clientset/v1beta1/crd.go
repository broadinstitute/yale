package v1beta1

import (
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type YaleCRDInterface interface {
	GcpSaKeys() GcpSaKeyInterface
	AzureClientSecrets() AzureClientSecretInterface
}

type YaleCRDClient struct {
	restClient rest.Interface
}

func NewForConfig(c *rest.Config) (*YaleCRDClient, error) {
	config := *c
	config.ContentConfig.GroupVersion = &schema.GroupVersion{Group: v1beta1.GroupName, Version: v1beta1.GroupVersion}
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	return &YaleCRDClient{restClient: client}, nil
}

// GcpSaKeys returns an interface for interacting with GCP SA keys
func (c *YaleCRDClient) GcpSaKeys() GcpSaKeyInterface {
	return &gcpsakeyClient{
		restClient: c.restClient,
	}
}

// AzureClientSecrets returns an interface for interacting with Azure client secrets
func (c *YaleCRDClient) AzureClientSecrets() AzureClientSecretInterface {
	return &azureClientSecretClient{
		restClient: c.restClient,
	}
}
