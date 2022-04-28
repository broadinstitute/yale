package yale

import (
	"context"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

func (m *Yale) DeleteKeys() error {
	// Get all GCPSaKey resources
	result, err := m.GetGCPSaKeyList()
	if err != nil {
		return err
	}
	secrets, gcpSaKeys, err := m.FilterRotatedKeys(result)
	for i, secret := range secrets {
		err = m.DeleteKey(secret, gcpSaKeys[i].Spec)
		if err != nil {
			return err
		}
	}
	return nil
}

// Removes 'oldServiceAccountKeyName' annotation
func (m *Yale) removeOldKeyName(K8Secret *corev1.Secret) error {
	// Restores annotations for secret
	annotations := K8Secret.GetAnnotations()
	delete(annotations, "oldServiceAccountKeyName")
	K8Secret.ObjectMeta.SetAnnotations(annotations)
	return m.UpdateSecret(K8Secret)
}

func (m *Yale) DeleteKey(k8Secret *corev1.Secret, gcpSaKeySpec apiv1b1.GCPSaKeySpec) error {
	keyName := k8Secret.Annotations["oldServiceAccountKeyName"]
	saKey, err := m.GetSAKey(keyName)
	if err != nil {
		return err
	}
	totalTime := gcpSaKeySpec.KeyRotation.DisableAfter + gcpSaKeySpec.KeyRotation.DeleteAfter
	isInUse, err := m.IsAuthenticated(totalTime, keyName, gcpSaKeySpec.GoogleServiceAccount.Project)
	if saKey.disabled && !isInUse {
		err = m.Delete(keyName)
		if err != nil {
			return err
		}
		return m.removeOldKeyName(k8Secret)
	}
	return nil
}

// Delete key
func (m *Yale) Delete(name string) error {
	ctx := context.Background()
	_, err := m.gcp.Projects.ServiceAccounts.Keys.Delete(name).Context(ctx).Do()
	return err
}

// GetSAKey Returns SA key
func (m *Yale) GetSAKey(keyName string) (*SaKey, error) {
	ctx := context.Background()
	saKey, err := m.gcp.Projects.ServiceAccounts.Keys.Get(keyName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return &SaKey{
		saKey.PrivateKeyData,
		saKey.Name,
		keyName,
		saKey.ValidAfterTime,
		saKey.Disabled,
	}, nil
}
