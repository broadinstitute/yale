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
	serviceAccountName := r.FindString(GCPSaKeySpec.GoogleServiceAccount.Name)
	key, err := m.GetSAKey(secretAnnotations["serviceAccountKeyName"], secretAnnotations["oldServiceAccountKeyName"])
	if err != nil {
		return err
	}
	if !key.disabled {
		canDisableKey, err := m.CanDisableKey(GCPSaKeySpec, key)
		if err != nil {
			return err
		}
		if canDisableKey {
			logs.Info.Printf("%s is allowed to be disabled.", r.FindString(serviceAccountName))
			logs.Info.Printf("Trying to disable %s.", r.FindString(serviceAccountName))
			err = m.Disable(secretAnnotations["oldServiceAccountKeyName"])
			if err != nil {
				return err
			}
		}
		logs.Info.Printf("%s is not allowed to be disabled.", r.FindString(serviceAccountName))
	}
	logs.Info.Printf("%s is already disabled.", r.FindString(serviceAccountName))
	return nil
}

// CanDisableKey Determines if a key can be disabled
func (m *Yale) CanDisableKey(GCPSaKeySpec apiv1b1.GCPSaKeySpec, key *SaKey) (bool, error) {
	logs.Info.Printf("Checking if %s can be disabled.", r.FindString(GCPSaKeySpec.GoogleServiceAccount.Name))
	keyIsNotUsed, err := m.IsAuthenticated(GCPSaKeySpec.KeyRotation.DisableAfter, key.serviceAccountKeyName, GCPSaKeySpec.GoogleServiceAccount.Project)
	if err != nil {
		return false, err
	}
	isTimeToDisable, err := IsExpired(key.validAfterTime, GCPSaKeySpec.KeyRotation.DisableAfter)
	if isTimeToDisable {
		logs.Info.Printf("Time to disable %s.", r.FindString(GCPSaKeySpec.GoogleServiceAccount.Name))
	}
	if err != nil {
		return false, err
	}
	return keyIsNotUsed && isTimeToDisable, err
}

// Disable key
func (m *Yale) Disable(keyName string) error {
	logs.Info.Printf("Disabling %s.", r.FindString(keyName))
	request := &iam.DisableServiceAccountKeyRequest{}
	ctx := context.Background()
	_, err := m.gcp.Projects.ServiceAccounts.Keys.Disable(keyName, request).Context(ctx).Do()
	return err
}

// IsAuthenticated Determines if key has been authenticated in x amount of days
func (m *Yale) IsAuthenticated(timeSinceAuth int, keyName string, googleProject string) (bool, error) {
	logs.Info.Printf("Checking if %s is being used.", r.FindString(keyName))
	saName := r.FindString(keyName)
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
	keyIsNotUsed, err := IsExpired(activity.LastAuthenticatedTime, timeSinceAuth)
	if keyIsNotUsed {
		logs.Info.Printf("%s is not being used.", saName)
	} else {
		logs.Info.Printf("%s is being used.", saName)
	}
	return keyIsNotUsed, err
}

// IsExpired Determines if it's time to disable a key
func IsExpired(beginDate string, duration int) (bool, error) {
	formatedBeginDate, err := time.Parse(time.RFC3339, beginDate)
	if err != nil {
		return false, err
	}
	// Date sa key expected to expire
	expireDate := formatedBeginDate.AddDate(0, 0, duration)
	if time.Now().After(expireDate) {
		return true, nil
	}
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
