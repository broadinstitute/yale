package yale

import (
	"encoding/base64"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/testing/gcp"
	"github.com/broadinstitute/yale/internal/yale/testing/k8s"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/policyanalyzer/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)
var activityResponse = policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse{
	Activities: []*policyanalyzer.GoogleCloudPolicyanalyzerV1Activity{
		{
			Activity:     googleapi.RawMessage(activity),
			ActivityType: "serviceAccountKeyLastAuthentication",
		}},
}
var activity = `{"lastAuthenticatedTime":"2021-04-18T07:00:00Z","serviceAccountKey":{"serviceAccountId":"108004111716625043518","projectNumber":"635957978953","fullResourceName":"//iam.googleapis.com/projects/broad-dsde-perf/serviceAccounts/agora-perf-service-account@broad-dsde-perf.iam.gserviceaccount.com/keys/e0b1b971487ffff7f725b124d3b729191f76b4cc"}}`

func TestDisableKeys(t *testing.T) {
	keyName := "my-sa@blah.com/keys/e0b1b971487ffff7f725b124d"

	testCases := []struct {
		name        string                                // set name of test case
		setupK8s    func(setup k8s.Setup)                 // add some fake objects to the cluster before test starts
		setupIam    func(expect gcp.ExpectIam)            // set up some mocked GCP api requests for the test
		setupPa     func(expect gcp.ExpectPolicyAnalyzer) // set up some mocked GCP api requests for the test
		verifyK8s   func(expect k8s.Expect)               // verify that the secrets we expect exist in the cluster after test completes
		expectError bool
	}{
		{
			name: "should disable key",
			setupK8s: func(setup k8s.Setup) {
				// Add a yale CRD to the fake cluster!
				// If we wanted, we could add some secrets here too with setup.AddSecret()
				setup.AddYaleCRD(v1beta1.GCPSaKey{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-gcp-sa-key",
						Namespace: "my-fake-namespace",
					},
					Spec: v1beta1.GCPSaKeySpec{
						GoogleServiceAccount: v1beta1.GoogleServiceAccount{
							Name:    "my-sa@blah.com",
							Project: "my-fake-project",
						},
						Secret: v1beta1.Secret{
							Name:        "my-fake-secret",
							PemKeyName:  "agora.pem",
							JsonKeyName: "agora.json",
						},
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  90,
							DisableAfter: 14,
							DeleteAfter:  7,
						},
					},
				})
				setup.AddSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-fake-secret",
						Namespace: "my-fake-namespace",
						UID:       "FakeUId",
						Annotations: map[string]string{
							"serviceAccountKeyName": newKeyName,
							"validAfterTime":        "2022-04-08T14:21:44Z",
							"oldServiceAccountKeyName": keyName,
						},
					},
					Data: map[string][]byte{
						"agora.pem":  []byte(NEW_FAKE_PEM),
						"agora.json": []byte(NEW_JSON_KEY),
					},
				})
			},
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", false).
					Returns(activityResponse)
			},
			setupIam: func(expect gcp.ExpectIam) {
				// set up a mock for a GCP api call to disable a service account
				expect.DisableServiceAccountKey("my-fake-project",  keyName).
					With(iam.DisableServiceAccountKeyRequest{}).
					Returns()

				expect.GetServiceAccountKey("my-fake-project", keyName).
					Returns(iam.ServiceAccountKey{
						Disabled:       false,
						Name:           newKeyName,
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(NEW_JSON_KEY)),
						ValidAfterTime: "2014-04-08T14:21:44Z",
						ServerResponse: googleapi.ServerResponse{},
					})

			},
			verifyK8s: func(expect k8s.Expect) {
				// set an expectation that a secret matching this one will exist in the cluster
				// once the test completes
				expect.HasSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-fake-secret",
						Namespace: "my-fake-namespace",
						UID:       "FakeUId",
						Annotations: map[string]string{
							"serviceAccountKeyName":    newKeyName,
							"serviceAccountName":       "my-sa@blah.com",
							"oldServiceAccountKeyName": keyName,
							"validAfterTime":           "2022-04-08T14:21:44Z",
						},
					},
					Data: map[string][]byte{
						"agora.pem":  []byte(NEW_FAKE_PEM),
						"agora.json": []byte(NEW_JSON_KEY),
					},
				})
			},
			expectError: false,
		},
		{
			name: "should disable key",
			setupK8s: func(setup k8s.Setup) {
				setup.AddYaleCRD(v1beta1.GCPSaKey{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-gcp-sa-key",
						Namespace: "my-fake-namespace",
					},
					Spec: v1beta1.GCPSaKeySpec{
						GoogleServiceAccount: v1beta1.GoogleServiceAccount{
							Name:    "my-sa@blah.com",
							Project: "my-fake-project",
						},
						Secret: v1beta1.Secret{
							Name:        "my-fake-secret",
							PemKeyName:  "agora.pem",
							JsonKeyName: "agora.json",
						},
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  90,
							DisableAfter: 14,
							DeleteAfter:  7,
						},
					},
				})
				setup.AddSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-fake-secret",
						Namespace: "my-fake-namespace",
						UID:       "FakeUId",
						Annotations: map[string]string{
							"serviceAccountKeyName": newKeyName,
							"validAfterTime":        "2022-04-08T14:21:44Z",
							"oldServiceAccountKeyName": keyName,
						},
					},
					Data: map[string][]byte{
						"agora.pem":  []byte(NEW_FAKE_PEM),
						"agora.json": []byte(NEW_JSON_KEY),
					},
				})
			},
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", false).
					Returns(activityResponse)
			},
			setupIam: func(expect gcp.ExpectIam) {
				// set up a mock for a GCP api call to disable a service account
				expect.DisableServiceAccountKey("my-fake-project",  keyName).
					With(iam.DisableServiceAccountKeyRequest{}).
					Returns()

				expect.GetServiceAccountKey("my-fake-project", keyName).
					Returns(iam.ServiceAccountKey{
						Disabled:       false,
						Name:           newKeyName,
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(NEW_JSON_KEY)),
						ValidAfterTime: "2014-04-08T14:21:44Z",
						ServerResponse: googleapi.ServerResponse{},
					})

			},
			verifyK8s: func(expect k8s.Expect) {
				// set an expectation that a secret matching this one will exist in the cluster
				// once the test completes
				expect.HasSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-fake-secret",
						Namespace: "my-fake-namespace",
						UID:       "FakeUId",
						Annotations: map[string]string{
							"serviceAccountKeyName":    newKeyName,
							"serviceAccountName":       "my-sa@blah.com",
							"oldServiceAccountKeyName": keyName,
							"validAfterTime":           "2022-04-08T14:21:44Z",
						},
					},
					Data: map[string][]byte{
						"agora.pem":  []byte(NEW_FAKE_PEM),
						"agora.json": []byte(NEW_JSON_KEY),
					},
				})
			},
			expectError: true,
		},

	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sMock := k8s.NewMock(tc.setupK8s, tc.verifyK8s)
			gcpMock := gcp.NewMock(tc.setupIam, tc.setupPa)

			gcpMock.Setup()
			t.Cleanup(gcpMock.Cleanup)

			clients := client.NewClients(gcpMock.GetIAMClient(), gcpMock.GetPAClient(), k8sMock.GetK8sClient(), k8sMock.GetYaleCRDClient())
			yale, err := NewYale(clients)
			require.NoError(t, err, "unexpected error constructing Yale")
			err = yale.DisableKeys()
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for %q, but err was nil", tc.name)
					return
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected errror for %v", tc.name)
				}
			}
			gcpMock.AssertExpectations(t)
			k8sMock.AssertExpectations(t)
		})
	}
}
