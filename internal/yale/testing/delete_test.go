package yale

import (
	yale2 "github.com/broadinstitute/yale/internal/yale"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/testing/gcp"
	"github.com/broadinstitute/yale/internal/yale/testing/k8s"
	"github.com/stretchr/testify/require"
	"testing"
)
var DeleteCrd = CRD
func TestDeleteKeys(t *testing.T) {

	testCases := []struct {
		name        string                                // set name of test case
		setupK8s    func(setup k8s.Setup)                 // add some fake objects to the cluster before test starts
		setupIam    func(expect gcp.ExpectIam)            // set up some mocked GCP api requests for the test
		setupPa     func(expect gcp.ExpectPolicyAnalyzer) // set up some mocked GCP api requests for the test
		verifyK8s   func(expect k8s.Expect)               // verify that the secrets we expect exist in the cluster after test completes
		expectError bool
	}{
		{
			name: "Show gracefully exit from Delete API returns error",
			setupK8s: func(setup k8s.Setup) {

				DeleteCrd.Spec.KeyRotation =
					v1beta1.KeyRotation{
						DeleteAfter:  3,
						DisableAfter: 14,
					}
				setup.AddYaleCRD(DeleteCrd)
				setup.AddSecret(newSecret)
			},
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", false).
					Returns(activityResponse)
			},
			setupIam: func(expect gcp.ExpectIam) {
				saKey.Disabled = true
				// set up a mock for a GCP api call to disable a service account
				expect.DeleteServiceAccountKey(OLD_KEY_NAME, true).
					Returns()

				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
					Returns(saKey)
			},
			verifyK8s: func(expect k8s.Expect) {
				expect.HasSecret(newSecret)
			},
			expectError: true,
		},
		{
			name: "Should delete key",
			setupK8s: func(setup k8s.Setup) {
				DeleteCrd.Spec.KeyRotation =
					v1beta1.KeyRotation{
						DeleteAfter:  3,
						DisableAfter: 14,
					}
				setup.AddYaleCRD(DeleteCrd)
				setup.AddSecret(newSecret)
			},
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", false).
					Returns(activityResponse)
			},
			setupIam: func(expect gcp.ExpectIam) {
				saKey.Disabled = true
				// set up a mock for a GCP api call to disable a service account
				expect.DeleteServiceAccountKey(OLD_KEY_NAME, false).
					Returns()

				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
					Returns(saKey)
			},
			verifyK8s: func(expect k8s.Expect) {
				newSecret.SetAnnotations(map[string]string{
					"validAfterDate":     "2022-04-08T14:21:44Z",
					"serviceAccountName": "my-sa@blah.com",
				})
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
			err = yale.DeleteKeys()
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
