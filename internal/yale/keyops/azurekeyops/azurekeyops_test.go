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

var testTenantIDB = "fake-tenant-id-b"
var testApplicationIDB = "asdf-asdf-asdfa-asdf-bbbb"
var unconfiguredTenantID = "unconfigured-tenant-id"

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

// Test_Create_MultiTenant_RoutesToConfiguredTenant is the "still picks the right client when
// more than one is configured" case: two tenants are configured, and a Create for each one
// must hit that tenant's own client, not the other's.
func Test_Create_MultiTenant_RoutesToConfiguredTenant(t *testing.T) {
	keyOps := setupTenants(t, []string{testTenantID, testTenantIDB}, func(expect msgraphmock.Expect) {
		expect.AddPassword(context.Background(), testApplicationID, msgraph.PasswordCredential{
			DisplayName: &testApplicationID,
		}).
			Returns(&msgraph.PasswordCredential{
				DisplayName: &testApplicationID,
				SecretText:  &testSecret,
				KeyId:       &testKeyID,
			})
		expect.AddPassword(context.Background(), testApplicationIDB, msgraph.PasswordCredential{
			DisplayName: &testApplicationIDB,
		}).
			Returns(&msgraph.PasswordCredential{
				DisplayName: &testApplicationIDB,
				SecretText:  &testSecret,
				KeyId:       &testKeyID,
			})
	})

	key, secret, err := keyOps.Create(testTenantID, testApplicationID)
	require.NoError(t, err)
	assert.Equal(t, testTenantID, key.Scope)
	assert.Equal(t, testApplicationID, key.Identifier)
	assert.Equal(t, testSecret, string(secret))

	keyB, secretB, err := keyOps.Create(testTenantIDB, testApplicationIDB)
	require.NoError(t, err)
	assert.Equal(t, testTenantIDB, keyB.Scope)
	assert.Equal(t, testApplicationIDB, keyB.Identifier)
	assert.Equal(t, testSecret, string(secretB))
}

func Test_Create_UnconfiguredTenant(t *testing.T) {
	keyOps := setup(t, func(expect msgraphmock.Expect) {})

	_, _, err := keyOps.Create(unconfiguredTenantID, testApplicationID)
	require.Error(t, err)
	assert.ErrorContains(t, err, "no Azure Graph client configured for tenant")
	assert.ErrorContains(t, err, unconfiguredTenantID)
}

func Test_IsDisabled_UnconfiguredTenant(t *testing.T) {
	keyOps := setup(t, func(expect msgraphmock.Expect) {})

	_, err := keyOps.IsDisabled(keyops.Key{Scope: unconfiguredTenantID, Identifier: testApplicationID, ID: testKeyID})
	require.Error(t, err)
	assert.ErrorContains(t, err, "no Azure Graph client configured for tenant")
}

func Test_DeleteIfDisabled_UnconfiguredTenant(t *testing.T) {
	keyOps := setup(t, func(expect msgraphmock.Expect) {})

	err := keyOps.DeleteIfDisabled(keyops.Key{Scope: unconfiguredTenantID, Identifier: testApplicationID, ID: testKeyID})
	require.Error(t, err)
	assert.ErrorContains(t, err, "no Azure Graph client configured for tenant")
}

func setup(t *testing.T, expectFn func(msgraphmock.Expect)) keyops.KeyOps {
	return setupTenants(t, []string{testTenantID}, expectFn)
}

// setupTenants wires up a single mock Graph client and registers it in the tenant->client map
// under every tenant ID given, so tests can exercise routing across more than one configured
// tenant. Note: msgraphmock's Cleanup() calls the jarcoal/httpmock package-level
// DeactivateAndReset(), which is global process state, not scoped to one mock instance --
// stacking multiple independent msgraphmock instances (each with their own Setup/Cleanup) in a
// single test causes one mock's Cleanup to wipe another's call-count tracking before it can
// assert. Sharing one mock across tenant keys avoids that entirely.
func setupTenants(t *testing.T, tenantIDs []string, expectFn func(msgraphmock.Expect)) keyops.KeyOps {
	mockMsGraph := msgraphmock.NewMockApplicationsClient(expectFn)
	mockMsGraph.Setup()

	t.Cleanup(func() {
		mockMsGraph.AssertExpectations(t)
		mockMsGraph.Cleanup()
	})

	clients := make(map[string]*msgraph.ApplicationsClient, len(tenantIDs))
	for _, tenantID := range tenantIDs {
		clients[tenantID] = mockMsGraph.GetClient()
	}
	return New(clients)
}
