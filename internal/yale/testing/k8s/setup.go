package k8s

import (
	apiv1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// Setup is used to populate the fake cluster with useful data before a test run
type Setup interface {
	// AddYaleCRD adds a Yale CRD to the fake cluster
	AddYaleCRD(crd apiv1.GCPSaKey)
	// AddSecret add a Secret to the fake cluster
	AddSecret(corev1.Secret)
}

func newSetup() *setup {
	return &setup{}
}

// implements Setup interface
type setup struct {
	yaleCrds []apiv1.GCPSaKey
	secrets  []corev1.Secret
}

func (s *setup) AddYaleCRD(yaleCrd apiv1.GCPSaKey) {
	s.yaleCrds = append(s.yaleCrds, yaleCrd)
}

func (s *setup) AddSecret(secret corev1.Secret) {
	s.secrets = append(s.secrets, secret)
}
