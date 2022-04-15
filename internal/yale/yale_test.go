package yale

import (
	"encoding/base64"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/testing/gcp"
	"github.com/broadinstitute/yale/internal/yale/testing/k8s"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iam/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

const FAKE_JSON_KEY = `{"private_key":"fake-sakey"}`
var FAKE_PEM =  "fake-sakey"
const NEW_JSON_KEY = `{"private_key": "newPrivateKeyData"}`
var NEW_FAKE_PEM = "newPrivateKeyData"
var newKeyName = "projects/my-fake-project/my-sa@blah.com/" + string(base64.StdEncoding.EncodeToString([]byte("newPrivateKeyData")))

func TestCreateGcpSaKeys(t *testing.T) {
	keyName := "projects/my-fake-project/my-sa@blah.com/e0b1b971487ffff7f725b124d"

	testCases := []struct {
		name      string                  // set name of test case
		setupK8s  func(setup k8s.Setup)   // add some fake objects to the cluster before test starts
		setupGcp  func(expect gcp.Expect) // set up some mocked GCP api requests for the test
		verifyK8s func(expect k8s.Expect) // verify that the secrets we expect exist in the cluster after test completes
		expectError bool
	}{
		{
			name: "should issue a new key if there is no existing secret for the CRD",

			setupK8s: func(setup k8s.Setup) {
				// Add a yale CRD to the fake cluster!
				// If we wanted, we could add some secrets here too with setup.AddSecret()
				setup.AddYaleCRD(v1beta1.GCPSaKey{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-gcp-sa-key",
						Namespace: "my-fake-namespace",
					},
					Spec: v1beta1.GCPSaKeySpec{
						GoogleServiceAccount: v1beta1.GoogleServiceAccount{
							Name:    "my-sa@blah.com",
							Project: "my-fake-project",
						},
						Secret:               v1beta1.Secret{
							Name:     "my-fake-secret",
							PemKeyName:     "agora.pem",
							JsonKeyName: "agora.json",
						},
						KeyRotation: v1beta1.KeyRotation{},
					},

				})
			},
			setupGcp: func(expect gcp.Expect) {
				// set up a mock for a GCP api call to create a service account
				expect.CreateServiceAccountKey("my-fake-project", "my-sa@blah.com").
					With(iam.CreateServiceAccountKeyRequest{
						KeyAlgorithm:   KEY_ALGORITHM,
						PrivateKeyType: KEY_FORMAT,
					}).
					Returns(iam.ServiceAccountKey{
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
					})
			},

			verifyK8s: func(expect k8s.Expect) {
				// set an expectation that a secret matching this one will exist in the cluster
				// once the test completes
				expect.HasSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-fake-secret",
						Namespace: "my-fake-namespace",
					},
					Data: map[string][]byte{
						"agora.json": []byte(FAKE_JSON_KEY),
						"agora.pem":  []byte(FAKE_PEM),
					},
				})
			},
			expectError : false,
		},
		{
			name: "should rotate key if original key is expired",

			setupK8s: func(setup k8s.Setup) {
				// Add a yale CRD to the fake cluster!
				// If we wanted, we could add some secrets here too with setup.AddSecret()
				setup.AddYaleCRD(v1beta1.GCPSaKey{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-gcp-sa-key",
						Namespace: "my-fake-namespace",
						UID: "FakeUId",
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
						KeyRotation: v1beta1.KeyRotation{},
					},
				})
				setup.AddSecret(corev1.Secret{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-fake-secret",
						Namespace: "my-fake-namespace",
						UID: "FakeUId",
						Annotations: map[string]string{
							"validAfterDate": "2022-04-08T14:21:44Z",
							"serviceAccountName": "my-sa@blah.com",
							"serviceAccountKeyName": keyName,
						},
					},
					Data: map[string][]byte{
						"agora.pem": []byte(FAKE_PEM),
						"agora.json": []byte(FAKE_JSON_KEY),
					},
				})
			},
			setupGcp: func(expect gcp.Expect) {
				expect.GetServiceAccountKey("my-fake-project", "my-sa@blah.com", keyName).
					Returns(iam.ServiceAccountKey{
					Disabled:        false,
					Name:            keyName,
					PrivateKeyData:  base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
					ValidAfterTime:  "2014-01-02T15:01:23Z",
				})
			expect.CreateServiceAccountKey("my-fake-project", "my-sa@blah.com").
				With(iam.CreateServiceAccountKeyRequest{
					KeyAlgorithm:   KEY_ALGORITHM,
					PrivateKeyType: KEY_FORMAT,
				}).
				Returns(iam.ServiceAccountKey{
					Name: newKeyName,
					PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(NEW_JSON_KEY)),
					ValidAfterTime: "2022-04-08T14:21:44Z",
				})
			},
			verifyK8s: func(expect k8s.Expect) {
				// set an expectation that a secret matching this one will exist in the cluster
				// once the test completes
				expect.HasSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-fake-secret",
						Namespace: "my-fake-namespace",
						UID: "FakeUId",
						Annotations: map[string]string{
							"serviceAccountKeyName": newKeyName,
							"oldKeyName" : keyName,
							"validAfterTime" : "2022-04-08T14:21:44Z",
						},
					},
					Data: map[string][]byte{
						"agora.pem":  []byte(NEW_FAKE_PEM),
						"agora.json": []byte(NEW_JSON_KEY),
					},
				})
			},
			expectError : false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sMock := k8s.NewMock(tc.setupK8s, tc.verifyK8s)
			gcpMock := gcp.NewMock(tc.setupGcp)

			gcpMock.Setup()
			t.Cleanup(gcpMock.Cleanup)

			clients := client.NewClients(gcpMock.GetIAMClient(), gcpMock.GetPAClient(), k8sMock.GetK8sClient(), k8sMock.GetYaleCRDClient())

			yale, err := NewYale(clients)
			require.NoError(t, err, "unexpected error constructing Yale")
			err = yale.RotateKeys() // TODO this should return errors so we can check for them :)
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for %q, but err was nil", tc.name)
					return
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected errror for %v", tc. name)
				}
			}
			gcpMock.AssertExpectations(t)
			k8sMock.AssertExpectations(t)
		})
	}
}

func TestDisableKeys(t *testing.T) {
	keyName := "projects/my-fake-project/my-sa@blah.com/e0b1b971487ffff7f725b124d"

	testCases := []struct {
		name      string                  // set name of test case
		setupK8s  func(setup k8s.Setup)   // add some fake objects to the cluster before test starts
		setupGcp  func(expect gcp.Expect) // set up some mocked GCP api requests for the test
		verifyK8s func(expect k8s.Expect) // verify that the secrets we expect exist in the cluster after test completes
		expectError bool
	}{{
		name: "should issue a new key if there is no existing secret for the CRD",

		setupK8s: func(setup k8s.Setup) {
			// Add a yale CRD to the fake cluster!
			// If we wanted, we could add some secrets here too with setup.AddSecret()
			setup.AddYaleCRD(v1beta1.GCPSaKey{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-gcp-sa-key",
					Namespace: "my-fake-namespace",
				},
				Spec: v1beta1.GCPSaKeySpec{
					GoogleServiceAccount: v1beta1.GoogleServiceAccount{
						Name:    "my-sa@blah.com",
						Project: "my-fake-project",
					},
					Secret:               v1beta1.Secret{
						Name:     "my-fake-secret",
						PemKeyName:     "agora.pem",
						JsonKeyName: "agora.json",
					},
					KeyRotation: v1beta1.KeyRotation{
						RotateAfter: 90,
						DisableAfter: 14,
						DeleteAfter: 7,
					},
				},

			})
			setup.AddSecret(corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-fake-secret",
					Namespace: "my-fake-namespace",
					UID: "FakeUId",
					Annotations: map[string]string{
						"serviceAccountKeyName": newKeyName,
						"oldKeyName" : keyName,
						"validAfterTime" : "2022-04-08T14:21:44Z",
					},
				},
				Data: map[string][]byte{
					"agora.pem":  []byte(NEW_FAKE_PEM),
					"agora.json": []byte(NEW_JSON_KEY),
				},
			})
			expect.GetServiceAccountKey("my-fake-project", "my-sa@blah.com", keyName).
				Returns(iam.ServiceAccountKey{
					Disabled:        false,
					Name:            keyName,
					PrivateKeyData:  base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
					ValidAfterTime:  "2014-01-02T15:01:23Z",
				})
		},
		setupGcp: func(expect gcp.Expect) {
			// set up a mock for a GCP api call to disable a service account
			expect.DisableServiceAccountKey("my-fake-project", "my-sa@blah.com", keyName).
				Returns(nil)
		},

		verifyK8s: func(expect k8s.Expect) {
			// set an expectation that a secret matching this one will exist in the cluster
			// once the test completes
			expect.HasSecret(corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-fake-secret",
					Namespace: "my-fake-namespace",
					UID: "FakeUId",
					Annotations: map[string]string{
						"serviceAccountKeyName" : newKeyName,
						"serviceAccountName" : "my-sa@blah.com",
						"oldServiceAccountKeyName" : keyName,
						"validAfterTime" : "2022-04-08T14:21:44Z",
						"oldKeyDisabled" = "true"
					},
				},
				Data: map[string][]byte{
					"agora.pem":  []byte(NEW_FAKE_PEM),
					"agora.json": []byte(NEW_JSON_KEY),
				},
			})
		},
		expectError : false,
	},}

}