package azurekeyops

import (
	"context"
	"testing"
	"time"

	"github.com/broadinstitute/yale/internal/yale/keyops"
	"github.com/broadinstitute/yale/internal/yale/keyops/azurekeyops/msgraphmock"
	"github.com/hashicorp/go-azure-sdk/sdk/odata"
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

var testKey = keyops.Key{
	Scope:      testTenantID,
	Identifier: testApplicationID,
	ID:         testKeyID,
}

var expiredTime = time.Now().Add(time.Hour * -24)

func Test_isDisabledTrue(t *testing.T) {
	keyops := setup(t, func(expect msgraphmock.Expect) {
		expect.Get(context.Background(), testApplicationID, odata.Query{}).
			Returns(&msgraph.Application{
				AppId: &testApplicationID,
				PasswordCredentials: &[]msgraph.PasswordCredential{
					{
						DisplayName: &testApplicationID,
						KeyId:       &testKeyID,
						SecretText:  &testSecret,
						EndDateTime: &expiredTime,
					},
				},
			})
	})
	disabled, err := keyops.IsDisabled(testKey)
	require.NoError(t, err)
	assert.True(t, disabled)

}

func Test_disableNonExistentKey(t *testing.T) {
	keyops := setup(t, func(expect msgraphmock.Expect) {
		expect.Get(context.Background(), testApplicationID, odata.Query{}).
			Returns(&msgraph.Application{
				AppId:               &testApplicationID,
				PasswordCredentials: &[]msgraph.PasswordCredential{},
			})
	})

	_, err := keyops.IsDisabled(testKey)
	require.ErrorContains(t, err, "error retrieving client secret info for application")

}

func Test_deleteIfDisabled(t *testing.T) {
	keyops := setup(t, func(expect msgraphmock.Expect) {
		expect.Get(context.Background(), testApplicationID, odata.Query{}).
			Returns(&msgraph.Application{
				AppId: &testApplicationID,
				PasswordCredentials: &[]msgraph.PasswordCredential{
					{
						DisplayName: &testApplicationID,
						KeyId:       &testKeyID,
						SecretText:  &testSecret,
						EndDateTime: &expiredTime,
					},
				},
			})
		expect.RemovePassword(context.Background(), testApplicationID, testKeyID).Returns()
	})

	err := keyops.DeleteIfDisabled(testKey)
	require.NoError(t, err)
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
