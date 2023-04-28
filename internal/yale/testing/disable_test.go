package yale

import (
	"encoding/base64"
	"encoding/json"
	yale2 "github.com/broadinstitute/yale/internal/yale"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/testing/gcp"
	"github.com/broadinstitute/yale/internal/yale/testing/k8s"
	"github.com/broadinstitute/yale/internal/yale/testing/vault"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/policyanalyzer/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

var secret = corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "my-fake-secret",
		Namespace: "my-fake-namespace",
		UID:       "FakeUId",
		Annotations: map[string]string{
			"serviceAccountKeyName":    keyName,
			"validAfterTime":           "2021-04-08T14:21:44Z",
			"oldServiceAccountKeyName": OLD_KEY_NAME,
		},
	},
	Data: map[string][]byte{
		"agora.pem":  []byte(NEW_FAKE_PEM),
		"agora.json": []byte(NEW_JSON_KEY),
	},
}

func TestDisableKeys(t *testing.T) {
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
			name: "Should retry on 420 error",
			crd: overrideDefaultCRD(func(crd *v1beta1.GCPSaKey) {
				crd.Spec.KeyRotation = v1beta1.KeyRotation{
					DisableAfter: 14,
				}
			}),
			secret: secret,
			// Policy analyzer returns 429
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", 429, &googleapi.Error{Code: 429, Message: "googleapi: Error 429: Quota exceeded for quota metric"}, 5).
					Returns(policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse{})
			},
			setupIam: func(expect gcp.ExpectIam) {
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
				expect.HasSecret(secret)
			},
			expectError: true,
		},
		{
			name: "Should disable key",
			crd: overrideDefaultCRD(func(crd *v1beta1.GCPSaKey) {
				crd.Spec.KeyRotation = v1beta1.KeyRotation{
					DisableAfter: 14,
				}
			}),
			secret: secret,
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", 200, nil, 1).
					Returns(hasAuthenticatedActivityResponse)
			},
			setupIam: func(expect gcp.ExpectIam) {
				// set up a mock for a GCP api call to disable a service account
				expect.DisableServiceAccountKey(OLD_KEY_NAME).
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
				expect.HasSecret(secret)
			},
			expectError: false,
		},
		{
			name: "Should not disable key before time to disable",
			crd: overrideDefaultCRD(func(crd *v1beta1.GCPSaKey) {
				crd.Spec.KeyRotation = v1beta1.KeyRotation{
					RotateAfter:  90,
					DisableAfter: 4000,
					DeleteAfter:  7,
				}
			}),
			secret: secret,
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
			},
			setupIam: func(expect gcp.ExpectIam) {
				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
					Returns(iam.ServiceAccountKey{
						Disabled:       false,
						Name:           OLD_KEY_NAME,
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
						ValidAfterTime: "4000-04-08T14:21:44Z",
						ServerResponse: googleapi.ServerResponse{},
					})

			},
			verifyK8s: func(expect k8s.Expect) {
				// set an expectation that a secret matching this one will exist in the cluster
				// once the test completes
				expect.HasSecret(secret)
			},
			expectError: false,
		},
		{
			name: "Should not disable key if error code != 200",
			crd: overrideDefaultCRD(func(crd *v1beta1.GCPSaKey) {
				crd.Spec.KeyRotation = v1beta1.KeyRotation{
					RotateAfter:  90,
					DisableAfter: 10,
					DeleteAfter:  7,
				}
			}),
			secret: secret,
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", 100, &googleapi.Error{Code: 400}, 1).
					Returns(policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse{})
			},
			setupIam: func(expect gcp.ExpectIam) {
				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
					Returns(iam.ServiceAccountKey{
						Disabled:       false,
						Name:           OLD_KEY_NAME,
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
						ValidAfterTime: "2020-04-08T14:21:44Z",
						ServerResponse: googleapi.ServerResponse{},
					})

			},
			verifyK8s: func(expect k8s.Expect) {
				// set an expectation that a secret matching this one will exist in the cluster
				// once the test completes
				expect.HasSecret(secret)
			},
			expectError: true,
		},
		{
			name: "Should throw error if there is no activity response",
			crd: overrideDefaultCRD(func(crd *v1beta1.GCPSaKey) {
				crd.Spec.KeyRotation = v1beta1.KeyRotation{
					RotateAfter:  90,
					DisableAfter: 10,
					DeleteAfter:  7,
				}
			}),
			secret: secret,
			setupPa: func(expect gcp.ExpectPolicyAnalyzer) {
				expect.CreateQuery("my-fake-project", 200, nil, 1).
					Returns(policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse{})
			},
			setupIam: func(expect gcp.ExpectIam) {
				expect.GetServiceAccountKey(OLD_KEY_NAME, false).
					Returns(iam.ServiceAccountKey{
						Disabled:       false,
						Name:           OLD_KEY_NAME,
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
						ValidAfterTime: "2020-04-08T14:21:44Z",
						ServerResponse: googleapi.ServerResponse{},
					})

			},
			verifyK8s: func(expect k8s.Expect) {
				// set an expectation that a secret matching this one will exist in the cluster
				// once the test completes
				expect.HasSecret(secret)
			},
			expectError: true,
		},
		{
			name: "Should not disable if the key is already disabled",
			crd: overrideDefaultCRD(func(crd *v1beta1.GCPSaKey) {
				crd.Spec.KeyRotation = v1beta1.KeyRotation{
					DisableAfter: 10,
				}
			}),
			secret:  secret,
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
				expect.HasSecret(secret)
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
			yale, err := yale2.NewYale(clients, func(options *yale2.Options) {
				options.PolicyAnalyzerRetrySleepTime = 500 * time.Millisecond
			})

			require.NoError(t, err, "unexpected error constructing Yale")
			err = yale.DisableKey(&tc.secret, tc.crd.Spec)
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

			k8sMock.AssertExpectations(t)
		})
	}
}

func overrideDefaultCRD(overrideFn func(gcpSaKey *v1beta1.GCPSaKey)) v1beta1.GCPSaKey {
	data, err := json.Marshal(CRD)
	if err != nil {
		panic(err)
	}
	var copy v1beta1.GCPSaKey
	if err = json.Unmarshal(data, &copy); err != nil {
		panic(err)
	}
	overrideFn(&copy)
	return copy
}
