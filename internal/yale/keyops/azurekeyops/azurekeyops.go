package azurekeyops

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/broadinstitute/yale/internal/yale/keyops"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"github.com/hashicorp/go-azure-sdk/sdk/odata"
	"github.com/manicminer/hamilton/msgraph"
)

type azKeyOps struct {
	applicationsClients map[string]*msgraph.ApplicationsClient
}

// New constructs a keyops.KeyOps for Azure AD application client secrets, given a Microsoft
// Graph client per tenant Yale is configured to manage secrets in, keyed by tenant ID.
func New(applicationsClients map[string]*msgraph.ApplicationsClient) keyops.KeyOps {
	return &azKeyOps{applicationsClients: applicationsClients}
}

// clientFor returns the Microsoft Graph client configured for the given tenant, or a loud,
// specific error if no credential is configured for that tenant -- a misconfigured CRD should
// fail visibly, not silently rotate against the wrong tenant (or the wrong app entirely).
func (a *azKeyOps) clientFor(tenantID string) (*msgraph.ApplicationsClient, error) {
	client, ok := a.applicationsClients[tenantID]
	if !ok {
		configured := make([]string, 0, len(a.applicationsClients))
		for t := range a.applicationsClients {
			configured = append(configured, t)
		}
		sort.Strings(configured)
		return nil, fmt.Errorf(
			"no Azure Graph client configured for tenant %q; Yale is configured for tenant(s): %s",
			tenantID, strings.Join(configured, ", "))
	}
	return client, nil
}

func (a *azKeyOps) Create(tenantID string, applicationID string) (keyops.Key, []byte, error) {
	client, err := a.clientFor(tenantID)
	if err != nil {
		return keyops.Key{}, nil, err
	}

	createKeyRequest := msgraph.PasswordCredential{
		DisplayName: &applicationID,
	}

	// Set a 30 second timeout for the request
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	// Ensure that the context is canceled to prevent leaking resources
	defer cancel()

	logs.Info.Printf("creating new client secret for application with id %s...", applicationID)
	createdKey, statusCode, err := client.AddPassword(ctx, applicationID, createKeyRequest)
	if err != nil {
		return keyops.Key{}, nil, fmt.Errorf(
			"error %d issuing new client secret for application with id %s: %v",
			statusCode, applicationID, err)
	}

	// ensure that the secretText field in the returned password credential is populated
	if createdKey.SecretText == nil {
		return keyops.Key{}, nil, fmt.Errorf(
			"error creating new client secret for application with id %s: secretText field was nil",
			applicationID)
	}

	// ensure that the keyId field in the returned password credential is populated
	if createdKey.KeyId == nil {
		return keyops.Key{}, nil, fmt.Errorf(
			"error creating new client secret for application with id %s: keyId field was nil",
			applicationID)
	}

	logs.Info.Printf("created new client secret for application with id %s", applicationID)
	clientSecretData := []byte(*createdKey.SecretText)
	return keyops.Key{
		Scope:      tenantID,
		Identifier: applicationID,
		ID:         *createdKey.KeyId,
	}, clientSecretData, nil
}

// Unlike GCP, in Azure there is no concept of a key that exists but is disabled.
// Instead we just check to see if the key exists and return true if so that yale's internal cache handling can still treat the key as disabled.
func (a *azKeyOps) IsDisabled(key keyops.Key) (bool, error) {
	client, err := a.clientFor(key.Scope)
	if err != nil {
		return false, err
	}

	applicationData, statusCode, err := client.Get(context.TODO(), key.Identifier, odata.Query{})
	if err != nil {
		return false, fmt.Errorf(
			"error %d retrieving client secret info for application %s failed : %v",
			statusCode, key.Identifier, err)
	}
	// ensure the passwordCredentials field is populated on the returned application
	if applicationData.PasswordCredentials == nil {
		return false, fmt.Errorf(
			"error retrieving client secret info for application %s: passwordCredentials field was nil",
			key.Identifier)
	}

	// iterate over the passwordCredentials field to find the credential with the matching keyId
	for _, credential := range *applicationData.PasswordCredentials {
		if credential.KeyId != nil && *credential.KeyId == key.ID {
			// Azure does not have the concept of a key that is disabled.
			// So here we just check to see if the key is a valid key that exists
			// and return true if so that yale's internal cache handling can appropriately treat the key as
			// disabled even the concept of a disabled client secret does not exist in Azure.
			return true, nil
		}
	}

	// if we get here, we didn't find a credential with the matching keyId
	return false, fmt.Errorf(
		"error retrieving client secret info for application %s: no credential found with keyId %s",
		key.Identifier, key.ID)
}

func (a *azKeyOps) EnsureDisabled(key keyops.Key) error {
	disabled, err := a.IsDisabled(key)
	if err != nil {
		return err
	}

	if disabled {
		logs.Info.Printf("client secret: %s for application with id %s in tenant %s is already disabled", key.ID, key.Identifier, key.Scope)
		return nil
	}

	logs.Info.Printf("client secret : %s for application with id %s in tenant %s is not disabled... skipping", key.ID, key.Identifier, key.Scope)
	return nil
}

func (a *azKeyOps) DeleteIfDisabled(key keyops.Key) error {
	disabled, err := a.IsDisabled(key)
	if err != nil {
		return err
	}

	if !disabled {
		return fmt.Errorf("client secret: %s for application with id %s in tenant %s is not disabled, cannot delete", key.ID, key.Identifier, key.Scope)
	}

	client, err := a.clientFor(key.Scope)
	if err != nil {
		return err
	}

	logs.Info.Printf("deleting client secret: %s for application with id %s in tenant %s", key.ID, key.Identifier, key.Scope)
	statusCode, err := client.RemovePassword(context.TODO(), key.Identifier, key.ID)
	if err != nil {
		return fmt.Errorf("error %d deleting client secret %s for application with id %s in tenant %s: %v", statusCode, key.ID, key.Identifier, key.Scope, err)
	}

	return nil
}
