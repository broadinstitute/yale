package keyops

import (
	"encoding/base64"
	mockiam "github.com/broadinstitute/yale/internal/yale/keyops/testutils/iam"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iam/v1"
	"testing"
)

const testProject = "my-project"
const testServiceAccount = "my-sa@my-project.iam.gserviceaccount.com"
const testKeyId = "my-key-id"

func Test_KeyCreate(t *testing.T) {
	ko := setup(t, func(expect mockiam.Expect) {
		expect.CreateServiceAccountKey(testProject, testServiceAccount).
			With(
				iam.CreateServiceAccountKeyRequest{
					KeyAlgorithm:   KEY_ALGORITHM,
					PrivateKeyType: KEY_FORMAT,
				},
			).Returns(
			iam.ServiceAccountKey{
				Name:           qualifiedKeyName(testProject, testServiceAccount, testKeyId),
				PrivateKeyData: base64.StdEncoding.EncodeToString([]byte(`{"foo":"bar"}`)),
			},
		)
	})

	key, data, err := ko.Create(testProject, testServiceAccount)
	require.NoError(t, err)

	assert.Equal(t, testProject, key.Project)
	assert.Equal(t, testServiceAccount, key.ServiceAccountEmail)
	assert.Equal(t, testKeyId, key.ID)
	assert.Equal(t, `{"foo":"bar"}`, string(data))
}

func Test_EnsureDisabledDisablesKeyIfEnabled(t *testing.T) {
	ko := setup(t, func(expect mockiam.Expect) {
		expect.GetServiceAccountKey(testProject, testServiceAccount, testKeyId).Returns(iam.ServiceAccountKey{
			Name:     qualifiedKeyName(testProject, testServiceAccount, testKeyId),
			Disabled: false,
		})
		expect.DisableServiceAccountKey(testProject, testServiceAccount, testKeyId).
			With(iam.DisableServiceAccountKeyRequest{}).Returns()
	})

	err := ko.EnsureDisabled(Key{
		Project:             testProject,
		ServiceAccountEmail: testServiceAccount,
		ID:                  testKeyId,
	})

	assert.NoError(t, err)
}

func Test_EnsureDisabledDoesNotDisableKeyIfAlreadyDisabled(t *testing.T) {
	ko := setup(t, func(expect mockiam.Expect) {
		expect.GetServiceAccountKey(testProject, testServiceAccount, testKeyId).Returns(iam.ServiceAccountKey{
			Name:     qualifiedKeyName(testProject, testServiceAccount, testKeyId),
			Disabled: true,
		})
	})
	err := ko.EnsureDisabled(Key{
		Project:             testProject,
		ServiceAccountEmail: testServiceAccount,
		ID:                  testKeyId,
	})
	assert.NoError(t, err)
}

func Test_Delete(t *testing.T) {
	ko := setup(t, func(expect mockiam.Expect) {
		expect.DeleteServiceAccountKey(testProject, testServiceAccount, testKeyId).Returns()
	})
	err := ko.Delete(Key{
		Project:             testProject,
		ServiceAccountEmail: testServiceAccount,
		ID:                  testKeyId,
	})
	assert.NoError(t, err)
}

func setup(t *testing.T, expectFn func(mockiam.Expect)) Keyops {
	mockIam := mockiam.NewMockIAMService(expectFn)

	mockIam.Setup()
	t.Cleanup(func() {
		mockIam.AssertExpectations(t)
		mockIam.Cleanup()
	})

	return New(mockIam.GetClient())
}
