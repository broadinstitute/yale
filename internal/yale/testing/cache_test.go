package yale

import (
	"encoding/json"
	"github.com/broadinstitute/yale/internal/yale"
	"github.com/broadinstitute/yale/internal/yale/cache"
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
	"time"
)

const threeDaysAgo = -3 * 24 * time.Hour

var createdAt = time.Now().Round(time.Second).Add(threeDaysAgo)

func TestPopulateCache(t *testing.T) {
	testCases := []struct {
		name        string                     // set name of test case
		setupK8s    func(setup k8s.Setup)      // add some fake objects to the cluster before test starts
		setupIam    func(expect gcp.ExpectIam) // set up some mocked GCP api requests for the test
		setupPA     func(analyzer gcp.ExpectPolicyAnalyzer)
		verifyK8s   func(expect k8s.Expect) // verify that the secrets we expect exist in the cluster after test completes
		verifyVault func(expect vault.Expect)
		expectError bool
	}{
		{
			name:    "should write key data to cache",
			setupPA: func(expect gcp.ExpectPolicyAnalyzer) {},
			setupK8s: func(setup k8s.Setup) {
				setup.AddYaleCRD(v1beta1.GCPSaKey{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sa1-spec",
						Namespace: "n1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						GoogleServiceAccount: v1beta1.GoogleServiceAccount{
							Project: "p1",
							Name:    "sa1@p1.com",
						},
						Secret: v1beta1.Secret{
							Name:        "sa1-key",
							JsonKeyName: "key.json",
						},
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  30,
							DisableAfter: 7,
							DeleteAfter:  7,
						},
					},
				})
				setup.AddYaleCRD(v1beta1.GCPSaKey{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sa2-spec",
						Namespace: "n2",
					},
					Spec: v1beta1.GCPSaKeySpec{
						GoogleServiceAccount: v1beta1.GoogleServiceAccount{
							Project: "p1",
							Name:    "sa2@p1.com",
						},
						Secret: v1beta1.Secret{
							Name:        "sa2-key",
							JsonKeyName: "key.json",
						},
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  30,
							DisableAfter: 7,
							DeleteAfter:  7,
						},
					},
				})
				setup.AddSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sa1-key",
						Namespace: "n1",
						Annotations: map[string]string{
							"serviceAccountKeyName":    "projects/p1/serviceAccounts/sa1@p1.com/keys/0002",
							"oldServiceAccountKeyName": "projects/p1/serviceAccounts/sa1@p1.com/keys/0001",
							"validAfterDate":           createdAt.Format(time.RFC3339),
						},
					},
					Data: map[string][]byte{
						"key.json": []byte("keydata1"),
					},
				})
				setup.AddSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sa2-key",
						Namespace: "n2",
						Annotations: map[string]string{
							"serviceAccountKeyName": "projects/p1/serviceAccounts/sa2@p1.com/keys/0001",
							"validAfterDate":        createdAt.Format(time.RFC3339),
						},
					},
					Data: map[string][]byte{
						"key.json": []byte("keydata2"),
					},
				})
			},
			setupIam: func(expect gcp.ExpectIam) {
				expect.GetServiceAccountKey("projects/p1/serviceAccounts/sa1@p1.com/keys/0001", false).
					Returns(iam.ServiceAccountKey{
						Disabled: false,
					})
			},
			verifyK8s: func(expect k8s.Expect) {
				expect.HasSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: cache.DefaultCacheNamespace,
						Name:      "yale-cache-sa1-p1.com",
						Annotations: map[string]string{
							"yale.terra.bio/cache-entry": "true",
						},
					},
					Data: map[string][]byte{
						"value": asJsonOrPanic(cache.Entry{
							ServiceAccount: struct {
								Email   string
								Project string
							}{
								Email:   "sa1@p1.com",
								Project: "p1",
							},
							CurrentKey: struct {
								JSON      string
								ID        string
								CreatedAt time.Time
							}{
								JSON:      "keydata1",
								ID:        "0002",
								CreatedAt: createdAt,
							},
							RotatedKeys: map[string]time.Time{
								"0001": createdAt,
							},
							DisabledKeys: map[string]time.Time{},
							SyncStatus:   map[string]string{},
						}),
					},
				})
				expect.HasSecret(corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: cache.DefaultCacheNamespace,
						Name:      "yale-cache-sa2-p1.com",
						Annotations: map[string]string{
							"yale.terra.bio/cache-entry": "true",
						},
					},
					Data: map[string][]byte{
						"value": asJsonOrPanic(cache.Entry{
							ServiceAccount: struct {
								Email   string
								Project string
							}{
								Email:   "sa2@p1.com",
								Project: "p1",
							},
							CurrentKey: struct {
								JSON      string
								ID        string
								CreatedAt time.Time
							}{
								JSON:      "keydata2",
								ID:        "0001",
								CreatedAt: createdAt,
							},
							RotatedKeys:  map[string]time.Time{},
							DisabledKeys: map[string]time.Time{},
							SyncStatus:   map[string]string{},
						}),
					},
				})
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
			err = yale.PopulateCache()
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

func asJsonOrPanic(e cache.Entry) []byte {
	data, err := json.Marshal(e)
	if err != nil {
		panic(err)
	}
	return data
}
