package yale

import (
	"context"
	apiv1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1"
	corev1 "k8s.io/api/core/v1"
)

func (m *Yale) DeleteKeys() error {
	// Get all GCPSaKey resources
	result, err := m.GetGCPSaKeyList()
	if err != nil {
		return err
	} else {
		secrets, gcpSaKeys := m.Filter(result)
		for i, secret := range secrets {
			keyName := secret.Annotations["oldServiceAccountKeyName"]
			saKey, err := m.GetSAKey(keyName, gcpSaKeys[i].Spec.GoogleProject)
			if err != nil {
				return err
			}
			spec := gcpSaKeys[i].Spec
			canDelete, err := m.CanDelete(spec, keyName)
			if saKey.disabled && canDelete {
				err = m.DeleteKey(keyName)
				if err != nil {
					return err
				}
				err = m.removeOldKeyName(secret, spec, *saKey)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Removes 'oldServiceAccountKeyName' annotation
func (m *Yale)removeOldKeyName(K8Secret *corev1.Secret, GCPSaKeySpec apiv1.GCPSaKeySpec, Key SaKey) error {
	// Restores annotations for secret
	newAnnotations := createAnnotations(Key)
	K8Secret.Annotations = newAnnotations
	err := m.UpdateSecret(GCPSaKeySpec, K8Secret)
	if err != nil {
		return err
	}
	return nil
}

// CanDelete Determines if key can be deleted
func (m *Yale)CanDelete( GCPSaKeySpec apiv1.GCPSaKeySpec, name string ) (bool, error){
	totalTime := GCPSaKeySpec.DaysDisabled+ GCPSaKeySpec.DaysDeauthenticated
	return m.isAuthenticated(totalTime, name, GCPSaKeySpec )
}

// DeleteKey Deletes key
func (m *Yale) DeleteKey(name string) error {
	ctx := context.Background()
	_, err := m.gcp.Projects.ServiceAccounts.Keys.Delete(name).Context(ctx).Do()
	return err
}

// GetSAKey Returns SA key
func (m *Yale) GetSAKey(name string, googleProject string)(*SaKey, error) {
	ctx := context.Background()
	saKey, err := m.gcp.Projects.ServiceAccounts.Keys.Get(name).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return &SaKey{
		googleProject,
		saKey.PrivateKeyData,
		saKey.Name,
		name,
		saKey.ValidAfterTime,
		saKey.Disabled,
	}, nil
}