package yale

import (
	"encoding/base64"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1"
	"github.com/broadinstitute/yale/internal/yale/testing/gcp"
	"github.com/broadinstitute/yale/internal/yale/testing/k8s"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iam/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestCreateSAKey(t *testing.T) {
	testCases := []struct {
		name      string                  // set name of test case
		setupK8s  func(setup k8s.Setup)   // add some fake objects to the cluster before test starts
		setupGcp  func(expect gcp.Expect) // set up some mocked GCP api requests for the test
		verifyK8s func(expect k8s.Expect) // verify that the secrets we expect exist in the cluster after test completes
		expectError bool				  // if function should return an error
	}{
		{
			name: "should issue a new key if there is no existing secret for the CRD",

			setupK8s: func(setup k8s.Setup) {
				// Add a yale CRD to the fake cluster!
				// If we wanted, we could add some secrets here too with setup.AddSecret()
				setup.AddYaleCRD(v1.GCPSaKey{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-gcp-sa-key",
					},
					Spec: v1.GCPSaKeySpec{
						GcpSaName:        "my-sa@blah.com",
						SecretName:       "my-fake-secret",
						Namespace:        "my-fake-namespace",
						PemDataFieldName: "pem",
						SecretDataKey:    "json",
						OlderThanDays:    30,
						GoogleProject:    "my-fake-project",
					},
				})
			},

			setupGcp: func(expect gcp.Expect) {
				// set up a mock for a GCP api call to create a service account
				expect.CreateServiceAccountKey("my-fake-project", "my-sa@blah.com").
					With(iam.CreateServiceAccountKeyRequest{
						KeyAlgorithm:   keyAlgorithm,
						PrivateKeyType: keyFormat,
					}).
					Returns(iam.ServiceAccountKey{
						PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(`{"private_key":"fake-sa-key"}`)),
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
						"json": []byte(`{"private_key":"fake-sa-key"}`),
						"pem":  []byte("fake-sa-key"),
					},
				})
			},
			expectError: false,
		},
		{
			name: "should return error when Google cloud return an error code",

			setupK8s: func(setup k8s.Setup) {
				// Add a yale CRD to the fake cluster!
				// If we wanted, we could add some secrets here too with setup.AddSecret()
				setup.AddYaleCRD(v1.GCPSaKey{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-gcp-sa-key",
					},
					Spec: v1.GCPSaKeySpec{
						GcpSaName:        "my-sa@blah.com",
						SecretName:       "my-fake-secret",
						Namespace:        "my-fake-namespace",
						PemDataFieldName: "pem",
						SecretDataKey:    "json",
						OlderThanDays:    30,
						GoogleProject:    "my-fake-project",
					},
				})
			},
		},
			{
				name: "Returns error when getSecret() error is unexpected",

				setupK8s: func(setup k8s.Setup) {
					// Add a yale CRD to the fake cluster!
					// If we wanted, we could add some secrets here too with setup.AddSecret()
					setup.AddYaleCRD(v1.GCPSaKey{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-gcp-sa-key",
						},
						Spec: v1.GCPSaKeySpec{
							GcpSaName:        "my-sa@blah.com",
							SecretName:       "my-fake-secret",
							Namespace:        "my-fake-namespace",
							PemDataFieldName: "pem",
							SecretDataKey:    "json",
							OlderThanDays:    30,
							GoogleProject:    "my-fake-project",
						},
					})
					setup.ReturnsError(errors.New("Bad Error"))
				},

				setupGcp: nil,
				verifyK8s: nil,
				expectError: true,
			},

		//	setupGcp: func(expect gcp.Expect) {
		//	// set up a mock for a GCP api call to create a service account
		//	expect.CreateServiceAccountKey("my-fake-project", "my-sa@blah.com").
		//		With(iam.CreateServiceAccountKeyRequest{
		//			KeyAlgorithm:   keyAlgorithm,
		//			PrivateKeyType: keyFormat,
		//		}).Returns(iam.ServiceAccountKey{
		//		PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(`{"private_key":"fake-sa-key"}`)),
		//	}).
		//		Status(404).ResponseBody(" Unknown service account, notFound")
		//	},
		//	verifyK8s: nil,
		//	expectError: true,
		//},
		//{
		//	name: "should issue a new key if there is no existing secret for the CRD",
		//
		//	setupK8s: func(setup k8s.Setup) {
		//		// Add a yale CRD to the fake cluster!
		//		// If we wanted, we could add some secrets here too with setup.AddSecret()
		//		setup.AddYaleCRD(v1.GCPSaKey{
		//			ObjectMeta: metav1.ObjectMeta{
		//				Name: "my-gcp-sa-key",
		//			},
		//			Spec: v1.GCPSaKeySpec{
		//				GcpSaName:        "my-sa@blah.com",
		//				SecretName:       "my-fake-secret",
		//				Namespace:        "my-fake-namespace",
		//				PemDataFieldName: "pem",
		//				SecretDataKey:    "json",
		//				OlderThanDays:    30,
		//				GoogleProject:    "my-fake-project",
		//			},
		//		})
		//	},

	}
		for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sMock := k8s.NewMock(tc.setupK8s, tc.verifyK8s)
			gcpMock := gcp.NewMock(tc.setupGcp)

			gcpMock.Setup()
			t.Cleanup(gcpMock.Cleanup)

			clients := client.NewClients(gcpMock.GetClient(), k8sMock.GetK8sClient(), k8sMock.GetYaleCRDClient())

			yale, err := NewYale(clients)
			require.NoError(t, err, "unexpected error constructing Yale")
			err = yale.GenerateKeys() // TODO this should return errors so we can check for them :)
			if tc.expectError {
				if err == nil {
					require.Errorf(t, err, "Expected error")
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
