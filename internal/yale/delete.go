package yale

import (
	"context"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	corev1 "k8s.io/api/core/v1"
)

// DeleteKeys Main method for deleting keys.
func (m *Yale) DeleteKeys() error {
	// Get all GCPSaKey resources
	result, err := m.GetGCPSaKeyList()
	if err != nil {
		return err
	}
	secrets, gcpSaKeys, err := m.FilterRotatedKeys(result)
	if err != nil {
		return err
	}
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
	annotations := K8Secret.GetAnnotations()
	delete(annotations, "oldServiceAccountKeyName")
	K8Secret.ObjectMeta.SetAnnotations(annotations)
	return m.UpdateSecret(K8Secret)
}

// DeleteKey Holds logic to delete a single key
func (m *Yale) DeleteKey(k8Secret *corev1.Secret, gcpSaKeySpec apiv1b1.GCPSaKeySpec) error {
	secretAnnotations := k8Secret.GetAnnotations()
	keyName := secretAnnotations["oldServiceAccountKeyName"]
	saName := secretAnnotations["serviceAccountKeyName"]
	saKey, err := m.GetSAKey(saName, keyName)
	keyNameForLogs := after(saKey.serviceAccountKeyName, "serviceAccounts/")
	if err != nil {
		return err
	}
	logs.Info.Printf("Checking if %s should be deleted.", keyNameForLogs)
	if saKey.disabled {
		totalTime := gcpSaKeySpec.KeyRotation.DisableAfter + gcpSaKeySpec.KeyRotation.DeleteAfter
		isExpired, err := IsExpired(secretAnnotations["validAfterDate"], totalTime)
		if err != nil {
			return err
		}
		if !isExpired {
			return nil
		}
		isNotUsed, err := m.IsNotAuthenticated(totalTime, keyName, gcpSaKeySpec.GoogleServiceAccount.Project)
		if err != nil {
			return err
		}
		if isNotUsed {
			logs.Info.Printf("%s can be deleted.", keyNameForLogs)
			err = m.Delete(keyName)
			if err != nil {
				return err
			}
			logs.Info.Printf("Successfully deleted %s.", keyNameForLogs)
			return m.removeOldKeyName(k8Secret)
		}
	} else {
		logs.Info.Printf("%s is not disabled yet and can not be deleted.", keyNameForLogs)
	}
	return nil
}

// Delete key
func (m *Yale) Delete(name string) error {
	logs.Info.Printf("Trying to delete %s.", after(name, "serviceAccounts/"))
	ctx := context.Background()
	_, err := m.gcp.Projects.ServiceAccounts.Keys.Delete(name).Context(ctx).Do()
	return err
}

// GetSAKey Returns an SA key
func (m *Yale) GetSAKey(saName string, keyName string) (*SaKey, error) {
	ctx := context.Background()
	saKey, err := m.gcp.Projects.ServiceAccounts.Keys.Get(keyName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return &SaKey{
		saKey.PrivateKeyData,
		saKey.Name,
		saName,
		saKey.ValidAfterTime,
		saKey.Disabled,
	}, nil
}
