package yale

import (
	"encoding/base64"
	yale2 "github.com/broadinstitute/yale/internal/yale"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/testing/gcp"
	"github.com/broadinstitute/yale/internal/yale/testing/k8s"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"testing"
)

func TestDisableKeys(t *testing.T) {

	testCases := []struct {
		name        string                                // set name of test case
		setupK8s    func(setup k8s.Setup)                 // add some fake objects to the cluster before test starts
		setupIam    func(expect gcp.ExpectIam)            // set up some mocked GCP api requests for the test
		setupPa     func(expect gcp.ExpectPolicyAnalyzer) // set up some mocked GCP api requests for the test
		verifyK8s   func(expect k8s.Expect)               // verify that the secrets we expect exist in the cluster after test completes
		expectError bool
	}{
		{
			name: "Should disable key",
			setupK8s: func(setup k8s.Setup) {
				CRD.Spec.KeyRotation =
					v1beta1.KeyRotation{

						DisableAfter: 14,
					}
				// Add a yale CRD to the fake cluster!
				// If we wanted, we could add some secrets here too with setup.AddSecret()
				setup.AddYaleCRD(CRD)
				setup.AddSecret(newSecret)
			},
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", false).
					Returns(activityResponse)
			},
			setupIam: func(expect gcp.ExpectIam) {
				// set up a mock for a GCP api call to disable a service account
				expect.DisableServiceAccountKey("my-fake-project", OLD_KEY_NAME).
					With(iam.DisableServiceAccountKeyRequest{}).
					Returns()

				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
					Returns(iam.ServiceAccountKey{
						Disabled:       false,
						Name:           OLD_KEY_NAME,
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
						ValidAfterTime: "2014-04-08T14:21:44Z",
						ServerResponse: googleapi.ServerResponse{},
					})

			},
			verifyK8s: func(expect k8s.Expect) {
				// set an expectation that a secret matching this one will exist in the cluster
				// once the test completes
				expect.HasSecret(newSecret)
			},
			expectError: false,
		},
		{
			name: "Should not disable key before time to disable",
			setupK8s: func(setup k8s.Setup) {
				CRD.Spec.KeyRotation =
					v1beta1.KeyRotation{
						RotateAfter:  90,
						DisableAfter: 4000,
						DeleteAfter:  7,
					}
				setup.AddYaleCRD(CRD)
				setup.AddSecret(newSecret)
			},
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", false).
					Returns(activityResponse)
			},
			setupIam: func(expect gcp.ExpectIam) {
				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
					Returns(iam.ServiceAccountKey{
						Disabled:       false,
						Name:           OLD_KEY_NAME,
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
						ValidAfterTime: "2023-04-08T14:21:44Z",
						ServerResponse: googleapi.ServerResponse{},
					})

			},
			verifyK8s: func(expect k8s.Expect) {
				// set an expectation that a secret matching this one will exist in the cluster
				// once the test completes
				expect.HasSecret(newSecret)
			},
			expectError: false,
		},
		{
			name: "Yale should gracefully throw error with bad policy analyzer API request",
			setupK8s: func(setup k8s.Setup) {
				CRD.Spec.KeyRotation =
					v1beta1.KeyRotation{
						RotateAfter:  90,
						DisableAfter: 200,
						DeleteAfter:  7,
					}
				setup.AddYaleCRD(CRD)
				setup.AddSecret(newSecret)
			},
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", true).
					Returns(activityResponse)
			},
			setupIam: func(expect gcp.ExpectIam) {
				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
					Returns(iam.ServiceAccountKey{
						Disabled:       false,
						Name:           OLD_KEY_NAME,
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
						ValidAfterTime: "2023-04-08T14:21:44Z",
						ServerResponse: googleapi.ServerResponse{},
					})

			},
			verifyK8s: func(expect k8s.Expect) {
				// set an expectation that a secret matching this one will exist in the cluster
				// once the test completes
				expect.HasSecret(newSecret)
			},
			expectError: true,
		},
		{
			name: "Should not disable if the key is already disabled",
			setupK8s: func(setup k8s.Setup) {
				CRD.Spec.KeyRotation =
					v1beta1.KeyRotation{
						DisableAfter: 10,
					}
				setup.AddYaleCRD(CRD)
				setup.AddSecret(newSecret)
			},
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {},
			setupIam: func(expect gcp.ExpectIam) {
				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
					Returns(iam.ServiceAccountKey{
						Disabled:       true,
						Name:           OLD_KEY_NAME,
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
						ValidAfterTime: "2021-04-08T14:21:44Z",
						ServerResponse: googleapi.ServerResponse{},
					})

			},
			verifyK8s: func(expect k8s.Expect) {
				// set an expectation that a secret matching this one will exist in the cluster
				// once the test completes
				expect.HasSecret(newSecret)
			},
			expectError: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sMock := k8s.NewMock(tc.setupK8s, tc.verifyK8s)
			gcpMock := gcp.NewMock(tc.setupIam, tc.setupPa)

			gcpMock.Setup()
			t.Cleanup(gcpMock.Cleanup)

			clients := client.NewClients(gcpMock.GetIAMClient(), gcpMock.GetPAClient(), k8sMock.GetK8sClient(), k8sMock.GetYaleCRDClient())
			yale, err := yale2.NewYale(clients)
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
