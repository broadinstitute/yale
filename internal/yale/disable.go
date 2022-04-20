package yale

import (
	"github.com/broadinstitute/yale/internal/yale/logs"
	"time"
)

//
//import (
//	"context"
//	"encoding/json"
//	"fmt"
//	apiv1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
//	"github.com/broadinstitute/yale/internal/yale/logs"
//	"google.golang.org/api/iam/v1beta1"
//	corev1 "k8s.io/api/core/v1beta1"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
//	"time"
//)
//
//type Activity struct {
//	LastAuthenticatedTime string            `json:"lastAuthenticatedTime"`
//	ServiceAccountKey     map[string]string `json:"serviceAccountKey"`
//}
//
//func (m *Yale) DisableKeys() error {
//	// Get all GCPSaKey resources
//	result, err := m.GetGCPSaKeyList()
//	if err != nil {
//		return err
//	} else {
//		secrets, gcpSaKeys := m.Filter(result)
//		for i, secret := range secrets {
//			// Check if time to disable key
//			keyName := secret.Annotations["oldServiceAccountKeyName"]
//			canDisableKey, err := m.isAuthenticated(gcpSaKeys[i].Spec.DaysDeauthenticated, keyName, gcpSaKeys[i].Spec)
//			if err != nil {
//				return err
//			}
//			if canDisableKey {
//				err = m.disableKey(keyName)
//				if err != nil {
//					return err
//				}
//			}
//		}
//		if err != nil {
//			return err
//		}
//	}
//
//	return err
//}
//
//
//// Disables key
//func (m *Yale) disableKey(name string) error {
//	request := &iam.DisableServiceAccountKeyRequest{}
//	ctx := context.Background()
//	_, err := m.gcp.Projects.ServiceAccounts.Keys.Disable(name, request).Context(ctx).Do()
//	return err
//}
//
////  Determines if key has been authenticated in x amount of days
//func (m *Yale) isAuthenticated(timeSinceAuth int, keyName string, GCPSaKeySpec apiv1.GCPSaKeySpec) (bool, error) {
//	query := fmt.Sprintf("projects/%s/locations/us-central1-a/activityTypes/serviceAccountKeyLastAuthentication", GCPSaKeySpec.GoogleProject)
//	queryFilter := fmt.Sprintf("activities.fullResourceName = \"//iam.googleapis.com/%s\"", keyName)
//	ctx := context.Background()
//	temp, err := m.gcpPA.Projects.Locations.ActivityTypes.Activities.Query(query).Filter(queryFilter).Context(ctx).Do()
//	activity := &Activity{}
//	results := temp.Activities[0]
//	json.Unmarshal(results.Activity, activity)
//	isTimeToDisable, err := m.IsTimeToDisable(activity.LastAuthenticatedTime, timeSinceAuth, keyName)
//	if err != nil {
//		return isTimeToDisable, err
//	}
//	return isTimeToDisable, nil
//}
//
//// Filter Filters gcpsakey that have the annotatiion 'oldServiceAccountKeyName'
//func (m *Yale) Filter(list *apiv1.GCPSaKeyList) ([]*corev1.Secret, []apiv1.GCPSaKey) {
//	var secrets []*corev1.Secret
//	var gcpSaKeys []apiv1.GCPSaKey
//	for _, gcpsakey := range list.Items {
//		secret, err := m.GetSecret(gcpsakey.Spec, metav1.GetOptions{})
//		if err != nil {
//			panic(err)
//		}
//		if _, ok := secret.Annotations["oldServiceAccountKeyName"]; ok {
//			secrets = append(secrets, secret)
//			gcpSaKeys = append(gcpSaKeys, gcpsakey)
//
//		}
//	}
//	return secrets, gcpSaKeys
//}
// IsTimeToDisable Determines if it's time to disable a key
func IsExpired(beginDate string, duration int, keyName string) (bool, error) {
	dateAuthorized, err := time.Parse("2006-01-02T15:04:05Z0700", beginDate)
	if err != nil {
		return false, err
	}
	// Date sa key expected to be expire
	expireDate := dateAuthorized.AddDate(0, 0, duration)
	if time.Now().After(expireDate) {
		logs.Info.Printf("Time for %v to be disabled", keyName)
		return true, nil
	}
	logs.Info.Printf("Not time for %v to be disabled", keyName)
	return false, nil
}