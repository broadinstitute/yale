package yale

import (
	"context"
	"encoding/json"
	"fmt"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/policyanalyzer/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
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
func after(value string, a string) string {
	// Get substring after a string.
	pos := strings.LastIndex(value, a)
	if pos == -1 {
		return ""
	}
	adjustedPos := pos + len(a)
	if adjustedPos >= len(value) {
		return ""
	}
	return value[adjustedPos:]
}
func (m *Yale) DisableKey(Secret *corev1.Secret, GCPSaKeySpec apiv1b1.GCPSaKeySpec) error {

	secretAnnotations := Secret.GetAnnotations()
	key, err := m.GetSAKey(secretAnnotations["serviceAccountKeyName"], secretAnnotations["oldServiceAccountKeyName"])
	keyNameForLogs := after(key.serviceAccountKeyName, "serviceAccounts/")
	if err != nil {
		return err
	}
	if !key.disabled {
		canDisableKey, err := m.CanDisableKey(GCPSaKeySpec, key)
		if err != nil {
			return err
		}
		if canDisableKey {
			logs.Info.Printf("%s is allowed to be disabled.", keyNameForLogs)
			logs.Info.Printf("Trying to disable %s.", keyNameForLogs)
			err = m.Disable(secretAnnotations["oldServiceAccountKeyName"])
			if err != nil {
				return err
			}
		} else {
			logs.Info.Printf("%s is not allowed to be disabled.", keyNameForLogs)
			return nil
		}
	} else {
		logs.Info.Printf("%s is already disabled.", keyNameForLogs)
	}
	return nil
}

// CanDisableKey Determines if a key can be disabled
func (m *Yale) CanDisableKey(GCPSaKeySpec apiv1b1.GCPSaKeySpec, key *SaKey) (bool, error) {
	var keyIsNotUsed = false
	keyNameForLogs := after(key.serviceAccountKeyName, "serviceAccounts/")
	logs.Info.Printf("Checking if %s can be disabled.", keyNameForLogs)
	isTimeToDisable, err := IsExpired(key.validAfterTime, GCPSaKeySpec.KeyRotation.DisableAfter)
	if err != nil {
		return false, err
	}
	if isTimeToDisable {
		logs.Info.Printf("Time to disable %s.", keyNameForLogs)
		keyIsNotUsed, err = m.IsNotAuthenticated(GCPSaKeySpec.KeyRotation.DisableAfter, key.serviceAccountKeyName, GCPSaKeySpec.GoogleServiceAccount.Project)
		if err != nil {
			return false, err
		}
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

// IsNotAuthenticated Determines if key has been authenticated in x amount of days
func (m *Yale) IsNotAuthenticated(timeSinceAuth int, keyName string, googleProject string) (bool, error) {
	activity := &Activity{}
	keyNameForLogs := after(keyName, "serviceAccounts/")
	logs.Info.Printf("Checking if %s is being used.", keyNameForLogs)
	query := fmt.Sprintf("projects/%s/locations/us-central1-a/activityTypes/serviceAccountKeyLastAuthentication", googleProject)
	queryFilter := fmt.Sprintf("activities.fullResourceName = \"//iam.googleapis.com/%s\"", keyName)
	ctx := context.Background()
	activityResp, err := m.gcpPA.Projects.Locations.ActivityTypes.Activities.Query(query).Filter(queryFilter).Context(ctx).Do()
	if err != nil {
		tooManyRequests := isfourTwentyNine(err.(*googleapi.Error))
		if !tooManyRequests {
			// another error occurred
			return false, err
		}
		activityResp, err = m.jitter(query, queryFilter)
		if err != nil {
			return false, err
		}
	}
	// There are no activities to report
	hasActivities, err := hasActivities(activityResp.Activities)
	if err != nil {
		return false, err
	}
	if hasActivities {
		results := activityResp.Activities[0]
		err = json.Unmarshal(results.Activity, activity)
		// Policy analyzer has a lag of ~2 days. When key has rotated recently, policy
		// analyzer will report an empty string because there are no authentication activity in the past 2 days
		if err != nil || len(activity.LastAuthenticatedTime) == 0 {
			logs.Info.Printf("%s has no authentication time to report.\n"+
				" Key likely rotated recently and is considered to be actively used.", keyNameForLogs)
			return false, err
		}
	}
	keyIsNotUsed, err := IsExpired(activity.LastAuthenticatedTime, timeSinceAuth)
	if keyIsNotUsed {
		logs.Info.Printf("%s is not being used.", keyNameForLogs)
	} else {
		logs.Info.Printf("%s is being used.", keyNameForLogs)
	}
	return keyIsNotUsed, err
}

func (m *Yale) jitter(query string, queryFilter string) (*policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse, error) {
	ctx := context.Background()
	var activityResp *policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse
	var err error
	for i := 1; i < 5; i++ {
		time.Sleep(15 * time.Second)
		activityResp, err = m.gcpPA.Projects.Locations.ActivityTypes.Activities.Query(query).Filter(queryFilter).Context(ctx).Do()
		if err == nil {
			return activityResp, err
		}
		tooManyRequests := isfourTwentyNine(err.(*googleapi.Error))
		if !tooManyRequests {
			return activityResp, err
		}
	}
	return activityResp, err
}

func hasActivities(activities []*policyanalyzer.GoogleCloudPolicyanalyzerV1Activity) (bool, error) {
	// There are no activities to report
	if activities == nil {
		logs.Error.Printf("No activities to report and there is no way to see when key was last authorized.")
		return false, fmt.Errorf("There are no activities to report. \n" +
			"Make sure policy analyzer enabled and exists.")
	}
	return true, nil
}
func isfourTwentyNine(err *googleapi.Error) bool {
	return (err.Code == 429) || strings.Contains(err.Message, "Quota exceeded for quota metric")
}

// IsExpired Determines if it's time to disable a key
func IsExpired(beginDate string, duration int) (bool, error) {
	formattedBeginDate, err := time.Parse(time.RFC3339, beginDate)
	if err != nil {
		return false, err
	}
	// Date sa key expected to expire
	expireDate := formattedBeginDate.AddDate(0, 0, duration)
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
