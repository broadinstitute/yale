package v1

import (
	"context"
	v1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

const endpoint = "gcpsakeys"

// GcpSaKeyInterface client interface fir interacting with GCP SA keys
type GcpSaKeyInterface interface {
	List(ctx context.Context, opts metav1.ListOptions) (*v1.GCPSaKeyList, error)
	Get(ctx context.Context, name string, options metav1.GetOptions) (*v1.GCPSaKey, error)
}

type gcpsakeyClient struct {
	restClient rest.Interface
}

func (c *gcpsakeyClient) List(ctx context.Context, opts metav1.ListOptions) (*v1.GCPSaKeyList, error) {
	result := v1.GCPSaKeyList{}
	err := c.restClient.
		Get().
		Resource(endpoint).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}

func (c *gcpsakeyClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.GCPSaKey, error) {
	result := v1.GCPSaKey{}
	err := c.restClient.
		Get().
		Resource(endpoint).
		Name(name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}
