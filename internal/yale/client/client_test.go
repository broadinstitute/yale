package client

import (
	"testing"

	"github.com/hashicorp/go-azure-sdk/sdk/environments"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_getYaleAppRegistrationCredentials_NotSet(t *testing.T) {
	_, err := getYaleAppRegistrationCredentials()
	require.Error(t, err)
	assert.ErrorContains(t, err, yaleAzureCredentialsEnvVar)
	assert.ErrorContains(t, err, "not set")
}

func Test_getYaleAppRegistrationCredentials_InvalidJSON(t *testing.T) {
	t.Setenv(yaleAzureCredentialsEnvVar, "not valid json")

	_, err := getYaleAppRegistrationCredentials()
	require.Error(t, err)
	assert.ErrorContains(t, err, yaleAzureCredentialsEnvVar)
}

func Test_getYaleAppRegistrationCredentials_Empty(t *testing.T) {
	t.Setenv(yaleAzureCredentialsEnvVar, "[]")

	_, err := getYaleAppRegistrationCredentials()
	require.Error(t, err)
	assert.ErrorContains(t, err, "at least one")
}

func Test_getYaleAppRegistrationCredentials_Valid(t *testing.T) {
	t.Setenv(yaleAzureCredentialsEnvVar, `[
		{"tenantId": "non-b2c-tenant-id", "clientId": "non-b2c-client-id"},
		{"tenantId": "b2c-tenant-id", "clientId": "b2c-client-id"}
	]`)

	credentials, err := getYaleAppRegistrationCredentials()
	require.NoError(t, err)
	assert.Equal(t, []azureAppRegistrationCredential{
		{TenantID: "non-b2c-tenant-id", ClientID: "non-b2c-client-id"},
		{TenantID: "b2c-tenant-id", ClientID: "b2c-client-id"},
	}, credentials)
}

// Test_getYaleAppRegistrationCredentials_LegacyFallback covers a not-yet-migrated deployment:
// the new JSON env var is absent, but the old single tenant/client pair is set.
func Test_getYaleAppRegistrationCredentials_LegacyFallback(t *testing.T) {
	t.Setenv(yaleLegacyTenantIDEnvVar, "legacy-tenant-id")
	t.Setenv(yaleLegacyClientIDEnvVar, "legacy-client-id")

	credentials, err := getYaleAppRegistrationCredentials()
	require.NoError(t, err)
	assert.Equal(t, []azureAppRegistrationCredential{
		{TenantID: "legacy-tenant-id", ClientID: "legacy-client-id"},
	}, credentials)
}

func Test_getYaleAppRegistrationCredentials_LegacyFallback_MissingClientID(t *testing.T) {
	t.Setenv(yaleLegacyTenantIDEnvVar, "legacy-tenant-id")

	_, err := getYaleAppRegistrationCredentials()
	require.Error(t, err)
	assert.ErrorContains(t, err, yaleLegacyClientIDEnvVar)
}

// Test_getYaleAppRegistrationCredentials_NewVarTakesPrecedence covers the case where both the
// new and legacy env vars happen to be set (e.g. mid-migration) -- the new one should win.
func Test_getYaleAppRegistrationCredentials_NewVarTakesPrecedence(t *testing.T) {
	t.Setenv(yaleAzureCredentialsEnvVar, `[{"tenantId": "new-tenant-id", "clientId": "new-client-id"}]`)
	t.Setenv(yaleLegacyTenantIDEnvVar, "legacy-tenant-id")
	t.Setenv(yaleLegacyClientIDEnvVar, "legacy-client-id")

	credentials, err := getYaleAppRegistrationCredentials()
	require.NoError(t, err)
	assert.Equal(t, []azureAppRegistrationCredential{
		{TenantID: "new-tenant-id", ClientID: "new-client-id"},
	}, credentials)
}

var testEnvironment = *environments.AzurePublic()

// Test_buildAzureCredentials_Local covers the bug this test was written to catch: TenantID
// must be set even for local (Azure CLI) auth. Left unset, the CLI authorizer falls back to
// `az account show`'s default tenant/subscription, which fails outright for identity-only
// tenants (e.g. Azure AD B2C) that have no subscription -- regardless of which tenant `az
// login` targeted.
func Test_buildAzureCredentials_Local(t *testing.T) {
	cred := azureAppRegistrationCredential{TenantID: "b2c-tenant-id", ClientID: "unused-client-id"}

	credentials := buildAzureCredentials(true, cred, "", testEnvironment)

	assert.Equal(t, "b2c-tenant-id", credentials.TenantID)
	assert.True(t, credentials.EnableAuthenticatingUsingAzureCLI)
	assert.False(t, credentials.EnableAuthenticationUsingOIDC)
	assert.Empty(t, credentials.ClientID)
	assert.Empty(t, credentials.OIDCAssertionToken)
}

// Test_buildAzureCredentials_Federated covers the in-cluster path: Yale authenticates as its
// own app registration via a GCP-issued OIDC token exchanged for an Azure token.
func Test_buildAzureCredentials_Federated(t *testing.T) {
	cred := azureAppRegistrationCredential{TenantID: "prod-tenant-id", ClientID: "yale-mgmt-client-id"}

	credentials := buildAzureCredentials(false, cred, "fake-oidc-token", testEnvironment)

	assert.Equal(t, "prod-tenant-id", credentials.TenantID)
	assert.Equal(t, "yale-mgmt-client-id", credentials.ClientID)
	assert.Equal(t, "fake-oidc-token", credentials.OIDCAssertionToken)
	assert.True(t, credentials.EnableAuthenticationUsingOIDC)
	assert.False(t, credentials.EnableAuthenticatingUsingAzureCLI)
}
