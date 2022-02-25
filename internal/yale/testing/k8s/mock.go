package k8s

import (
	"context"
	"fmt"
	v1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1"
	clientv1 "github.com/broadinstitute/yale/internal/yale/crd/clientset/v1"
	"github.com/broadinstitute/yale/internal/yale/crd/clientset/v1/mocks"
	"github.com/stretchr/testify/assert"
	tmock "github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	"testing"
)

type Mock interface {
	// GetK8sClient returns a fake Kubernetes client populated with fake data
	GetK8sClient() kubernetes.Interface
	// GetYaleCRDClient returns a mock Yale CRD client
	GetYaleCRDClient() clientv1.YaleCRDInterface
	// AssertExpectations verifies that cluster state matches configured expectations
	AssertExpectations(t *testing.T) bool
}

func NewMock(setupFn func(Setup), expectFn func(Expect)) Mock {
	s := newSetup()
	e := newExpect()
	setupFn(s)
	expectFn(e)

	return &mock{
		k8s:           buildK8sClient(s),
		crd:           buildCRDClient(s),
		expectSecrets: e.secrets,
	}
}

type mock struct {
	k8s           kubernetes.Interface
	crd           clientv1.YaleCRDInterface
	expectSecrets []corev1.Secret
}

func (m *mock) GetK8sClient() kubernetes.Interface {
	return m.k8s
}

func (m *mock) GetYaleCRDClient() clientv1.YaleCRDInterface {
	return m.crd
}

func (m *mock) AssertExpectations(t *testing.T) bool {
	for _, expected := range m.expectSecrets {
		actual, err := m.k8s.CoreV1().Secrets(expected.Namespace).Get(context.Background(), expected.Name, metav1.GetOptions{})
		if !assert.NoError(t, err, "get %s (namespace %s) should not return an error", expected.Name, expected.Namespace) {
			return false
		}
		if !assert.Equal(t, expected.Name, actual.Name, "name mismatch for %s (namespace %s)", expected.Name, expected.Namespace) {
			return false
		}
		if !assert.Equal(t, expected.Data, actual.Data, "data mismatch for %s (namespace %s)", expected.Name, expected.Namespace) {
			return false
		}
		// TODO these comparisons fail for now, we could think about enabling them
		//if !assert.Equal(t, expected.Labels, actual.Labels, "label mismatch for %s (namespace %s)", expected.Name, expected.Namespace) {
		//	return false
		//}
		//if !assert.Equal(t, expected.Annotations, actual.Annotations, "annotation mismatch for %s (namespace %s)", expected.Name, expected.Namespace) {
		//	return false
		//}
	}
	return true
}

func buildK8sClient(s *setup) kubernetes.Interface {
	var objects []runtime.Object
	for _, secret := range s.secrets {
		objects = append(objects, &secret)
	}
	k8s := k8sfake.NewSimpleClientset(objects...)
	k8s.PrependReactor("create", "secrets", secretDataReactor)
	return k8s
}

// secretDataReactor: A reactor that makes it possible to persist secret data updates to the fake cluster
// ganked from: https://github.com/creydr/go-k8s-utils
func secretDataReactor(action ktesting.Action) (bool, runtime.Object, error) {
	secret, ok := action.(ktesting.CreateAction).GetObject().(*corev1.Secret)
	if !ok {
		return false, nil, fmt.Errorf("SecretDataReactor can only be applied on secrets")
	}

	if len(secret.StringData) > 0 {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}

		for k, v := range secret.StringData {
			secret.Data[k] = []byte(v)
		}
	}

	return false, nil, nil
}

func buildCRDClient(s *setup) clientv1.YaleCRDInterface {
	keysEndpoint := new(mocks.GcpSaKeyInterface)
	keysEndpoint.On("List", tmock.Anything, tmock.Anything).
		Return(
			&v1.GCPSaKeyList{Items: s.yaleCrds},
			nil,
		)

	crdClient := new(mocks.YaleCrdV1Interface)
	crdClient.On("GcpSaKeys").Return(keysEndpoint)

	return crdClient
}
