package yale

import (
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *Yale) Run() error {
	list, err := m.GetGCPSaKeyList()
	if err != nil {
		return err
	}

	for _, gsk := range list.Items {
		if err = m.processKey(gsk); err != nil {
			return err
		}
	}

	return m.PopulateCache()
}

func (m *Yale) processKey(gsk apiv1b1.GCPSaKey) error {
	secret, err := m.RotateKey(gsk)
	if err != nil {
		return err
	}
	if !metav1.HasAnnotation(secret.ObjectMeta, "oldServiceAccountKeyName") {
		// there is no old key to disable or delete
		return nil
	}
	if err := m.DisableKey(secret, gsk.Spec); err != nil {
		return err
	}
	if err := m.DeleteKey(secret, gsk.Spec); err != nil {
		return err
	}

	return nil
}
