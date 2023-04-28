package yale

import (
	"encoding/base64"
	yale2 "github.com/broadinstitute/yale/internal/yale"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/testing/gcp"
	"github.com/broadinstitute/yale/internal/yale/testing/k8s"
	"github.com/broadinstitute/yale/internal/yale/testing/vault"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

var DeleteCrd = CRD

func TestDeleteKeys(t *testing.T) {

	testCases := []struct {
		name        string // set name of test case
		crd         v1beta1.GCPSaKey
		secret      corev1.Secret
		setupIam    func(expect gcp.ExpectIam)            // set up some mocked GCP api requests for the test
		setupPa     func(expect gcp.ExpectPolicyAnalyzer) // set up some mocked GCP api requests for the test
		verifyK8s   func(expect k8s.Expect)               // verify that the secrets we expect exist in the cluster after test completes
		expectError bool
	}{
		{
			name: "Should not delete key before time to",
			crd: overrideDefaultCRD(func(crd *v1beta1.GCPSaKey) {
				crd.Spec.KeyRotation = v1beta1.KeyRotation{
					DisableAfter: 14,
					DeleteAfter:  2,
				}
			}),
			secret: newSecret,
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
			},
			setupIam: func(expect gcp.ExpectIam) {
				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
					Returns(iam.ServiceAccountKey{
						Disabled:       true,
						Name:           OLD_KEY_NAME,
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
						ValidAfterTime: "4000-04-08T14:21:44Z",
						ServerResponse: googleapi.ServerResponse{},
					})
			},
			verifyK8s: func(expect k8s.Expect) {
				expect.HasSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-fake-secret",
						Namespace: "my-fake-namespace",
						UID:       "FakeUId",
						Annotations: map[string]string{
							"serviceAccountKeyName": keyName,
							"validAfterTime":        "4000-04-08T14:21:44Z",
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
			name: "Show gracefully exit from Delete API returns error",
			crd: overrideDefaultCRD(func(crd *v1beta1.GCPSaKey) {
				crd.Spec.KeyRotation = v1beta1.KeyRotation{
					DisableAfter: 14,
					DeleteAfter:  3,
				}
			}),
			secret: expiredSecret,
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", 200, nil, 1).
					Returns(hasAuthenticatedActivityResponse)
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
				expect.HasSecret(expiredSecret)
			},
			expectError: true,
		},
		{
			name: "Should delete key",
			crd: overrideDefaultCRD(func(crd *v1beta1.GCPSaKey) {
				crd.Spec.KeyRotation = v1beta1.KeyRotation{
					DisableAfter: 14,
					DeleteAfter:  3,
				}
			}),
			secret: expiredSecret,
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", 200, nil, 1).
					Returns(hasAuthenticatedActivityResponse)
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
				expect.HasSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-fake-secret",
						Namespace: "my-fake-namespace",
						UID:       "FakeUId",
						Annotations: map[string]string{
							"serviceAccountKeyName": keyName,
							"validAfterTime":        "2000-04-08T14:21:44Z",
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
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sMock := k8s.NewMock(func(setup k8s.Setup) {
				setup.AddYaleCRD(tc.crd)
				setup.AddSecret(tc.secret)
			}, tc.verifyK8s)
			iamMock := gcp.NewIamMock(tc.setupIam)
			paMock := gcp.NewPolicyAnaylzerMock(tc.setupPa)
			fakeVault := vault.NewFakeVaultServer(t, nil)

			iamMock.Setup()
			paMock.Setup()
			t.Cleanup(iamMock.Cleanup)
			t.Cleanup(paMock.Cleanup)

			clients := client.NewClients(iamMock.GetClient(), paMock.GetClient(), k8sMock.GetK8sClient(), k8sMock.GetYaleCRDClient(), fakeVault.NewClient())
			yale, err := yale2.NewYale(clients)
			require.NoError(t, err, "unexpected error constructing Yale")
			err = yale.DeleteKey(&tc.secret, tc.crd.Spec)
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
			iamMock.AssertExpectations(t)
			paMock.AssertExpectations(t)
			k8sMock.AssertExpectations(t)
		})
	}

}
