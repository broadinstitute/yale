package azurekeyops

import (
	"context"
	"testing"

	"github.com/broadinstitute/yale/internal/yale/keyops"
	"github.com/broadinstitute/yale/internal/yale/keyops/azurekeyops/msgraphmock"
	"github.com/manicminer/hamilton/msgraph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testApplicationID = "asdf-asdf-asdfa-asdf-asdf"
var testTenantID = "fake-tenant-id"
var testSecret = "test-secret"
var testKeyID = "test-key-id"

func Test_Create(t *testing.T) {
	keyOps := setup(t, func(expect msgraphmock.Expect) {
		expect.AddPassword(context.Background(), testApplicationID, msgraph.PasswordCredential{
			DisplayName: &testApplicationID,
		}).
			Returns(&msgraph.PasswordCredential{
				DisplayName: &testApplicationID,
				SecretText:  &testSecret,
				KeyId:       &testKeyID,
			})
	})

	key, secret, err := keyOps.Create(testTenantID, testApplicationID)
	require.NoError(t, err)

	assert.Equal(t, testTenantID, key.Scope)
	assert.Equal(t, testApplicationID, key.Identifier)
	assert.Equal(t, testKeyID, key.ID)
	assert.Equal(t, testSecret, string(secret))
}

func Test_CreateErrorsIfResponseLacksKeyID(t *testing.T) {
	keyOps := setup(t, func(expect msgraphmock.Expect) {
		expect.AddPassword(context.Background(), testApplicationID, msgraph.PasswordCredential{
			DisplayName: &testApplicationID,
		}).
			Returns(&msgraph.PasswordCredential{
				DisplayName: &testApplicationID,
				SecretText:  &testSecret,
			})
	})

	_, _, err := keyOps.Create(testTenantID, testApplicationID)
	require.Error(t, err)
	assert.ErrorContains(t, err, "keyId field was nil")
}

func Test_CreateErrorsIfResponseLacksSecret(t *testing.T) {
	keyOps := setup(t, func(expect msgraphmock.Expect) {
		expect.AddPassword(context.Background(), testApplicationID, msgraph.PasswordCredential{
			DisplayName: &testApplicationID,
		}).
			Returns(&msgraph.PasswordCredential{
				DisplayName: &testApplicationID,
				KeyId:       &testKeyID,
			})
	})

	_, _, err := keyOps.Create(testTenantID, testApplicationID)
	require.Error(t, err)
	assert.ErrorContains(t, err, "secretText field was nil")
}

func setup(t *testing.T, expectFn func(msgraphmock.Expect)) keyops.KeyOps {
	mockMsGraph := msgraphmock.NewMockApplicationsClient(expectFn)
	mockMsGraph.Setup()

	t.Cleanup(func() {
		mockMsGraph.AssertExpectations(t)
		mockMsGraph.Cleanup()
	})
	return New(mockMsGraph.GetClient())
}
