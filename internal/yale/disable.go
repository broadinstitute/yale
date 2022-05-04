package yale

import (
	"context"
	"encoding/json"
	"fmt"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"google.golang.org/api/iam/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
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


func (m *Yale) DisableKey(Secret *corev1.Secret, GCPSaKeySpec apiv1b1.GCPSaKeySpec) error {
	secretAnnotations := Secret.GetAnnotations()
	key, err := m.GetSAKey(GCPSaKeySpec.GoogleServiceAccount.Project, secretAnnotations["oldServiceAccountKeyName"])
	if err != nil {
		return err
	}
	if !key.disabled {
		canDisableKey, err := m.CanDisableKey(GCPSaKeySpec, key)
		if err != nil {
			return err
		}
		if canDisableKey {
			err = m.Disable(GCPSaKeySpec.GoogleServiceAccount.Project, secretAnnotations["oldServiceAccountKeyName"])
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// CanDisableKey Determines if a key can be disabled
func (m *Yale) CanDisableKey(GCPSaKeySpec apiv1b1.GCPSaKeySpec, key *SaKey) (bool, error) {
	keyIsInUse, err := m.IsAuthenticated(GCPSaKeySpec.KeyRotation.DisableAfter, key.serviceAccountKeyName, GCPSaKeySpec.GoogleServiceAccount.Project)
	if err != nil {
		return false, err
	}
	isTimeToDisable, err := IsExpired(key.validAfterTime, GCPSaKeySpec.KeyRotation.DisableAfter, key.serviceAccountKeyName)
	if err != nil {
		return false, err
	}
	return !keyIsInUse && isTimeToDisable, err
}

// Disable key
func (m *Yale) Disable(googleProject string, keyName string) error {
	name := fmt.Sprintf("projects/%s/serviceAccounts/%s", googleProject, keyName)
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
	activityResp, err := m.gcpPA.Projects.Locations.ActivityTypes.Activities.Query(query).Filter(queryFilter).Context(ctx).Do()
	if err != nil {
		return false, err
	}
	// There are no activities to report or key has not been authenticated against in the past 2 days
	if activityResp.Activities == nil {
		return false, nil
	}
	activity := &Activity{}
	results := activityResp.Activities[0]
	err = json.Unmarshal(results.Activity, activity)
	if err != nil {
		return false, err
	}
	isTimeToDisable, err := IsExpired(activity.LastAuthenticatedTime, timeSinceAuth, keyName)
	return !isTimeToDisable, err
}

// IsExpired Determines if it's time to disable a key
func IsExpired(beginDate string, duration int, keyName string) (bool, error) {
	dateAuthorized, err := time.Parse(time.RFC3339, beginDate)
	if err != nil {
		return false, err
	}
	// Date sa key expected to be expire
	expireDate := dateAuthorized.AddDate(0, 0, duration)
	if time.Now().After(expireDate) {
		logs.Info.Printf("%v has not expired", keyName)
		return true, nil
	}
	logs.Info.Printf("%v has expired", keyName)
	return false, nil
}

// FilterRotatedKeys Returns secrets that have rotated, which contain annotation 'oldServiceAccountKeyName' and their GSK resource
func (m *Yale) FilterRotatedKeys(list *apiv1b1.GCPSaKeyList) ([]*corev1.Secret, []apiv1b1.GCPSaKey, error) {
	var secrets []*corev1.Secret
	var gcpSaKeys []apiv1b1.GCPSaKey
	for _, gsk := range list.Items {
		secret, err := m.GetSecret(gsk.Spec.Secret, gsk.Namespace)
		if err != nil {
			return nil, nil, err
		}
		if metav1.HasAnnotation(secret.ObjectMeta, "oldServiceAccountKeyName") {
			secrets = append(secrets, secret)
			gcpSaKeys = append(gcpSaKeys, gsk)
		}
	}
	return secrets, gcpSaKeys, nil
}
