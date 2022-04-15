package yale

import (
	"context"
	"encoding/json"
	"fmt"
	apiv1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"google.golang.org/api/iam/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Activity struct {
	LastAuthenticatedTime string            `json:"lastAuthenticatedTime"`
	ServiceAccountKey     map[string]string `json:"serviceAccountKey"`
}

func (m *Yale) DisableKeys() error {
	// Get all GCPSaKey resource
	result, err := m.GetGCPSaKeyList()
	if err != nil {
		return err
	}
	secrets, gcpSaKeys, err := m.FilterRotatedKeys(result)
	if err != nil {
		return err
	}
	for i, secret := range secrets {
		err = m.DisableKey(secret, gcpSaKeys[i].Spec)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Yale) DisableKey(Secret *corev1.Secret, GCPSaKeySpec apiv1.GCPSaKeySpec)error{
	secretAnnotations := Secret.GetAnnotations()
	key := createKeyFromAnnotations(secretAnnotations)
	canDisableKey, err := m.CanDisableKey(GCPSaKeySpec, key)
	if err != nil {
		return err
	}
	if canDisableKey {
		err = m.Disable(key.serviceAccountKeyName)
		if err != nil {
			return err
		}
		// Add annotation that says old key is disabled
		secretAnnotations["oldKeyDisabled"] = "true"
		Secret.ObjectMeta.SetAnnotations(secretAnnotations)
	}
	return nil
}

func createKeyFromAnnotations(annotations map[string]string)*SaKey{
	return &SaKey{
		"",
		annotations["serviceAccountKeyName"],
		annotations["serviceAccountName"],
		annotations["validAfterDate"],
		false,
	}
}

func (m *Yale) CanDisableKey(GCPSaKeySpec apiv1.GCPSaKeySpec, key *SaKey)(bool, error){
	keyIsInUse, err := m.IsAuthenticated( GCPSaKeySpec.KeyRotation.DisableAfter, key.serviceAccountKeyName, GCPSaKeySpec.GoogleServiceAccount.Project )
	if err != nil {
		return false, err
	}
	isTimeToDisable, err := IsExpired(key.validAfterTime, GCPSaKeySpec.KeyRotation.DisableAfter, key.serviceAccountKeyName )
	if err != nil {
		return false, err
	}
	return !keyIsInUse && isTimeToDisable, err
}

// Disable key
func (m *Yale) Disable(name string) error {
	request := &iam.DisableServiceAccountKeyRequest{}
	ctx := context.Background()
	_, err := m.gcp.Projects.ServiceAccounts.Keys.Disable(name, request).Context(ctx).Do()
	return err
}

// IsAuthenticated Determines if key has been authenticated in x amount of days
func (m *Yale) IsAuthenticated(timeSinceAuth int, keyName string, googleProject string) (bool, error) {
	query := fmt.Sprintf("projects/%s/locations/us-central1-a/activityTypes/serviceAccountKeyLastAuthentication", googleProject)
	queryFilter := fmt.Sprintf("activities.fullResourceName = \"//iam.googleapis.com/%s\"", keyName)
	ctx := context.Background()
	temp, err := m.gcpPA.Projects.Locations.ActivityTypes.Activities.Query(query).Filter(queryFilter).Context(ctx).Do()
	activity := &Activity{}
	results := temp.Activities[0]
	err = json.Unmarshal(results.Activity, activity)
	if err != nil {
		return false, err
	}
	isTimeToDisable, err := IsExpired(activity.LastAuthenticatedTime, timeSinceAuth, keyName)
	return isTimeToDisable, err
}

// FilterRotatedKeys Returns secrets that have rotated, which contain annotation 'oldServiceAccountKeyName' and their GSK resource
func (m *Yale) FilterRotatedKeys(list *apiv1.GCPSaKeyList) ([]*corev1.Secret, []apiv1.GCPSaKey, error) {
	var secrets []*corev1.Secret
	var gcpSaKeys []apiv1.GCPSaKey
	for _, gsk := range list.Items {
		secret, err := m.GetSecret(gsk.Spec.Secret, gsk.Namespace)
		if err != nil {
			return nil, nil, err
		}
		if metav1.HasAnnotation(secret.ObjectMeta,"oldServiceAccountKeyName"){
			secrets = append(secrets, secret)
			gcpSaKeys = append(gcpSaKeys, gsk)
		}
	}
	return secrets, gcpSaKeys, nil
}
