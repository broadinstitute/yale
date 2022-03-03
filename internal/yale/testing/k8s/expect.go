package k8s

import corev1 "k8s.io/api/core/v1"

// Expect is used to set expectations / verifications for the fake K8s cluster that will be checked after a test run
type Expect interface {
	// HasSecret will verify that the fake cluster has a secret matching the given configuration
	HasSecret(corev1.Secret)
}

func newExpect() *expect {
	return &expect{}
}

// implements Expect interface
type expect struct {
	secrets []corev1.Secret
}

func (e *expect) HasSecret(secret corev1.Secret) {
	e.secrets = append(e.secrets, secret)
}

