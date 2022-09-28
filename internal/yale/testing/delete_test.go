package yale

//
//import (
//	"encoding/base64"
//	yale2 "github.com/broadinstitute/yale/internal/yale"
//	"github.com/broadinstitute/yale/internal/yale/client"
//	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
//	"github.com/broadinstitute/yale/internal/yale/testing/gcp"
//	"github.com/broadinstitute/yale/internal/yale/testing/k8s"
//	"github.com/stretchr/testify/require"
//	"google.golang.org/api/googleapi"
//	"google.golang.org/api/iam/v1"
//	corev1 "k8s.io/api/core/v1"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//	"testing"
//)
//
//var DeleteCrd = CRD
//
//func TestDeleteKeys(t *testing.T) {
//
//	testCases := []struct {
//		name        string                                // set name of test case
//		setupK8s    func(setup k8s.Setup)                 // add some fake objects to the cluster before test starts
//		setupIam    func(expect gcp.ExpectIam)            // set up some mocked GCP api requests for the test
//		setupPa     func(expect gcp.ExpectPolicyAnalyzer) // set up some mocked GCP api requests for the test
//		verifyK8s   func(expect k8s.Expect)               // verify that the secrets we expect exist in the cluster after test completes
//		expectError bool
//	}{
//		{
//			name: "Should not delete key before time to",
//			setupK8s: func(setup k8s.Setup) {
//				DeleteCrd.Spec.KeyRotation =
//					v1beta1.KeyRotation{
//						DeleteAfter:  2500,
//						DisableAfter: 14,
//					}
//				setup.AddYaleCRD(DeleteCrd)
//				setup.AddSecret(newSecret)
//			},
//			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
//				expect.CreateQuery("my-fake-project", false).
//					Returns(hasAuthenticatedActivityResponse)
//			},
//			setupIam: func(expect gcp.ExpectIam) {
//				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
//					Returns(iam.ServiceAccountKey{
//						Disabled:       true,
//						Name:           OLD_KEY_NAME,
//						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
//						ValidAfterTime: "2022-04-08T14:21:44Z",
//						ServerResponse: googleapi.ServerResponse{},
//					})
//			},
//			verifyK8s: func(expect k8s.Expect) {
//				expect.HasSecret(corev1.Secret{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "my-fake-secret",
//						Namespace: "my-fake-namespace",
//						UID:       "FakeUId",
//						Annotations: map[string]string{
//							"serviceAccountKeyName": keyName,
//							"validAfterTime":        "2021-04-08T14:21:44Z",
//						},
//					},
//					Data: map[string][]byte{
//						"agora.pem":  []byte(NEW_FAKE_PEM),
//						"agora.json": []byte(NEW_JSON_KEY),
//					},
//				})
//			},
//			expectError: false,
//		},
//		//{
//		//	name: "Show gracefully exit from Delete API returns error",
//		//	setupK8s: func(setup k8s.Setup) {
//		//
//		//		DeleteCrd.Spec.KeyRotation =
//		//			v1beta1.KeyRotation{
//		//				DeleteAfter:  3,
//		//				DisableAfter: 14,
//		//			}
//		//		setup.AddYaleCRD(DeleteCrd)
//		//		setup.AddSecret(newSecret)
//		//	},
//		//	setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
//		//		expect.CreateQuery("my-fake-project", false).
//		//			Returns(hasAuthenticatedActivityResponse)
//		//	},
//		//	setupIam: func(expect gcp.ExpectIam) {
//		//		saKey.Disabled = true
//		//		// set up a mock for a GCP api call to disable a service account
//		//		expect.DeleteServiceAccountKey(OLD_KEY_NAME, true).
//		//			Returns()
//		//
//		//		expect.GetServiceAccountKey(OLD_KEY_NAME, false).
//		//			Returns(saKey)
//		//	},
//		//	verifyK8s: func(expect k8s.Expect) {
//		//		expect.HasSecret(newSecret)
//		//	},
//		//	expectError: true,
//		//},
//		{
//			name: "Should delete key",
//			setupK8s: func(setup k8s.Setup) {
//				DeleteCrd.Spec.KeyRotation =
//					v1beta1.KeyRotation{
//						DeleteAfter:  3,
//						DisableAfter: 14,
//					}
//				setup.AddYaleCRD(DeleteCrd)
//				setup.AddSecret(newSecret)
//			},
//			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
//				expect.CreateQuery("my-fake-project", false).
//					Returns(hasAuthenticatedActivityResponse)
//			},
//			setupIam: func(expect gcp.ExpectIam) {
//				saKey.Disabled = true
//				// set up a mock for a GCP api call to disable a service account
//				expect.DeleteServiceAccountKey(OLD_KEY_NAME, false).
//					Returns()
//
//				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
//					Returns(saKey)
//			},
//			verifyK8s: func(expect k8s.Expect) {
//				newSecret.SetAnnotations(map[string]string{
//					"validAfterDate":     "2022-04-08T14:21:44Z",
//					"serviceAccountName": "my-sa@blah.com",
//				})
//				expect.HasSecret(corev1.Secret{
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "my-fake-secret",
//						Namespace: "my-fake-namespace",
//						UID:       "FakeUId",
//						Annotations: map[string]string{
//							"serviceAccountKeyName": keyName,
//							"validAfterTime":        "2021-04-08T14:21:44Z",
//						},
//					},
//					Data: map[string][]byte{
//						"agora.pem":  []byte(NEW_FAKE_PEM),
//						"agora.json": []byte(NEW_JSON_KEY),
//					},
//				})
//			},
//			expectError: false,
//		},
//	}
//	for _, tc := range testCases {
//		t.Run(tc.name, func(t *testing.T) {
//			k8sMock := k8s.NewMock(tc.setupK8s, tc.verifyK8s)
//			gcpMock := gcp.NewMock(tc.setupIam, tc.setupPa)
//
//			gcpMock.Setup()
//			t.Cleanup(gcpMock.Cleanup)
//
//			clients := client.NewClients(gcpMock.GetIAMClient(), gcpMock.GetPAClient(), k8sMock.GetK8sClient(), k8sMock.GetYaleCRDClient())
//			yale, err := yale2.NewYale(clients)
//			require.NoError(t, err, "unexpected error constructing Yale")
//			err = yale.DeleteKeys()
//			if tc.expectError {
//				if err == nil {
//					t.Errorf("Expected error for %q, but err was nil", tc.name)
//					return
//				}
//			} else {
//				if err != nil {
//					t.Errorf("Unexpected errror for %v", tc.name)
//				}
//			}
//			gcpMock.AssertExpectations(t)
//			k8sMock.AssertExpectations(t)
//		})
//	}
//
//}
