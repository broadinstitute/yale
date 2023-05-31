package v1beta1

import (
	"context"

	v1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

const azEndpoint = "azureClientSecrets"

type AzureClientSecretInterface interface {
	List(ctx context.Context, opts metav1.ListOptions) (*v1.AzureClientSecretList, error)
	Get(ctx context.Context, name string, options metav1.GetOptions) (*v1.AzureClientSecret, error)
}

type azureClientSecretClient struct {
	restClient rest.Interface
}

func (c *azureClientSecretClient) List(ctx context.Context, opts metav1.ListOptions) (*v1.AzureClientSecretList, error) {
	result := v1.AzureClientSecretList{}
	err := c.restClient.
		Get().
		Resource(azEndpoint).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}

func (c *azureClientSecretClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.AzureClientSecret, error) {
	result := v1.AzureClientSecret{}
	err := c.restClient.
		Get().
		Resource(azEndpoint).
		Name(name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}
