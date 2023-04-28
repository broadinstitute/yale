package yale

import (
	"encoding/base64"
	"github.com/broadinstitute/yale/internal/yale"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/testing/gcp"
	"github.com/broadinstitute/yale/internal/yale/testing/k8s"
	"github.com/broadinstitute/yale/internal/yale/testing/vault"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iam/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestRotateKeys(t *testing.T) {
	newKeyName := "projects/my-fake-project/my-sa@blah.com/e0b1b971487ffff7f725b124h"

	testCases := []struct {
		name        string // set name of test case
		crd         v1beta1.GCPSaKey
		setupK8s    func(setup k8s.Setup)      // add some fake objects to the cluster before test starts
		setupIam    func(expect gcp.ExpectIam) // set up some mocked GCP api requests for the test
		setupPA     func(analyzer gcp.ExpectPolicyAnalyzer)
		verifyK8s   func(expect k8s.Expect) // verify that the secrets we expect exist in the cluster after test completes
		verifyVault func(expect vault.Expect)
		expectError bool
	}{
		{
			name:    "should issue a new key if there is no existing secret for the CRD",
			setupPA: func(expect gcp.ExpectPolicyAnalyzer) {},
			crd:     CRD,
			setupIam: func(expect gcp.ExpectIam) {
				// set up a mock for a GCP api call to create a service account
				expect.CreateServiceAccountKey("my-fake-project", "my-sa@blah.com", false).
					With(iam.CreateServiceAccountKeyRequest{
						KeyAlgorithm:   yale.KEY_ALGORITHM,
						PrivateKeyType: yale.KEY_FORMAT,
					}).
					Returns(iam.ServiceAccountKey{
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(FAKE_JSON_KEY)),
					})
			},
			verifyK8s: func(expect k8s.Expect) {
				expect.HasSecret(OLD_SECRET)
			},
			expectError: false,
		},
		{
			name: "should replicate key to Vault if spec has vault replications",
			crd: v1beta1.GCPSaKey{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-key",
					Namespace: "foo-namespace",
					UID:       "FakeUId",
				},
				Spec: v1beta1.GCPSaKeySpec{
					GoogleServiceAccount: v1beta1.GoogleServiceAccount{
						Name:    "foo-sa@blah.com",
						Project: "foo-project",
					},
					Secret: v1beta1.Secret{
						Name:        "foo-sa-secret",
						PemKeyName:  "sa.pem",
						JsonKeyName: "sa.json",
					},
					KeyRotation: v1beta1.KeyRotation{
						RotateAfter: 45000,
					},
					VaultReplications: []v1beta1.VaultReplication{
						{
							Path:   "secret/foo/test/map",
							Format: v1beta1.Map,
						},
						{
							Path:   "secret/foo/test/json",
							Format: v1beta1.JSON,
							Key:    "key.json",
						},
						{
							Path:   "secret/foo/test/base64",
							Format: v1beta1.Base64,
							Key:    "key.b64",
						},
						{
							Path:   "secret/foo/test/pem",
							Format: v1beta1.PEM,
							Key:    "key.pem",
						},
					},
				},
			},
			setupPA: func(expect gcp.ExpectPolicyAnalyzer) {},
			setupIam: func(expect gcp.ExpectIam) {
				// set up a mock for a GCP api call to create a service account key
				expect.CreateServiceAccountKey("foo-project", "foo-sa@blah.com", false).
					With(iam.CreateServiceAccountKeyRequest{
						KeyAlgorithm:   yale.KEY_ALGORITHM,
						PrivateKeyType: yale.KEY_FORMAT,
					}).
					Returns(iam.ServiceAccountKey{
						Name:           "foo-sa@blah.com",
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(`{"email":"foo-sa@blah.com","private_key":"new-foo-key"}`)),
					})
			},
			verifyK8s: func(expect k8s.Expect) {
				expect.HasSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-sa-secret",
						Namespace: "foo-namespace",
					},
					Data: map[string][]byte{
						"sa.pem":  []byte("new-foo-key"),
						"sa.json": []byte(`{"email":"foo-sa@blah.com","private_key":"new-foo-key"}`),
					},
				})
			},
			verifyVault: func(expect vault.Expect) {
				expect.HasSecret("secret/foo/test/map", map[string]interface{}{
					"email":       "foo-sa@blah.com",
					"private_key": "new-foo-key",
				})
				expect.HasSecret("secret/foo/test/json", map[string]interface{}{
					"key.json": `{"email":"foo-sa@blah.com","private_key":"new-foo-key"}`,
				})
				expect.HasSecret("secret/foo/test/base64", map[string]interface{}{
					"key.b64": "eyJlbWFpbCI6ImZvby1zYUBibGFoLmNvbSIsInByaXZhdGVfa2V5IjoibmV3LWZvby1rZXkifQ==",
				})
				expect.HasSecret("secret/foo/test/pem", map[string]interface{}{
					"key.pem": "new-foo-key",
				})
			},
			expectError: false,
		},
		{
			name:    "should rotate key if original key is expired",
			setupPA: func(expect gcp.ExpectPolicyAnalyzer) {},
			crd:     CRD,
			setupK8s: func(setup k8s.Setup) {
				setup.AddSecret(corev1.Secret{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-fake-secret",
						Namespace: "my-fake-namespace",
						UID:       "FakeUId",
						Annotations: map[string]string{
							"validAfterDate":        "2022-04-08T14:21:44Z",
							"serviceAccountName":    "my-sa@blah.com",
							"serviceAccountKeyName": OLD_KEY_NAME,
						},
					},
					Data: map[string][]byte{
						"agora.pem":  []byte(FAKE_PEM),
						"agora.json": []byte(FAKE_JSON_KEY),
					},
				})
			},
			setupIam: func(expect gcp.ExpectIam) {
				expect.CreateServiceAccountKey("my-fake-project", "my-sa@blah.com", false).
					With(iam.CreateServiceAccountKeyRequest{
						KeyAlgorithm:   yale.KEY_ALGORITHM,
						PrivateKeyType: yale.KEY_FORMAT,
					}).
					Returns(iam.ServiceAccountKey{
						Name:           newKeyName,
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(NEW_JSON_KEY)),
						ValidAfterTime: "2022-04-08T14:21:44Z",
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
			name: "Yale should gracefully throw error with bad request",
			crd:  CRD,
			setupK8s: func(setup k8s.Setup) {
				setup.AddSecret(OLD_SECRET)
			},
			setupPA: func(expect gcp.ExpectPolicyAnalyzer) {},
			setupIam: func(expect gcp.ExpectIam) {
				expect.CreateServiceAccountKey("my-fake-project", "my-sa@blah.com", true).
					With(iam.CreateServiceAccountKeyRequest{
						KeyAlgorithm:   yale.KEY_ALGORITHM,
						PrivateKeyType: yale.KEY_FORMAT,
					}).
					Returns(iam.ServiceAccountKey{})

			},
			verifyK8s: func(expect k8s.Expect) {
				expect.HasSecret(OLD_SECRET)
			},
			expectError: true,
		},
		{
			name: "Secret should remain the same when key is not rotated",
			crd: v1beta1.GCPSaKey{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gcp-sa-key",
					Namespace: "my-fake-namespace",
					UID:       "FakeUId",
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
						RotateAfter: 45000,
					},
				},
			},
			setupPA: func(expect gcp.ExpectPolicyAnalyzer) {},
			setupK8s: func(setup k8s.Setup) {
				setup.AddSecret(newSecret)
			},
			setupIam: func(expect gcp.ExpectIam) {},
			verifyK8s: func(expect k8s.Expect) {
				expect.HasSecret(newSecret)
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sMock := k8s.NewMock(tc.setupK8s, tc.verifyK8s)
			iamMock := gcp.NewIamMock(tc.setupIam)
			paMock := gcp.NewPolicyAnaylzerMock(tc.setupPA)
			fakeVault := vault.NewFakeVaultServer(t, tc.verifyVault)

			iamMock.Setup()
			paMock.Setup()
			t.Cleanup(iamMock.Cleanup)
			t.Cleanup(paMock.Cleanup)

			clients := client.NewClients(iamMock.GetClient(), paMock.GetClient(), k8sMock.GetK8sClient(), k8sMock.GetYaleCRDClient(), fakeVault.NewClient())

			yale, err := yale.NewYale(clients)
			require.NoError(t, err, "unexpected error constructing Yale")
			_, err = yale.RotateKey(tc.crd)
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for %q, but err was nil", tc.name)
					return
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
			iamMock.AssertExpectations(t)
			paMock.AssertExpectations(t)
			k8sMock.AssertExpectations(t)
			fakeVault.AssertExpectations(t)
		})
	}
}
