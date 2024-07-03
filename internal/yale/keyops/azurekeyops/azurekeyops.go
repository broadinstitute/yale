package azurekeyops

import (
	"context"
	"fmt"
	"time"

	"github.com/broadinstitute/yale/internal/yale/keyops"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"github.com/hashicorp/go-azure-sdk/sdk/odata"
	"github.com/manicminer/hamilton/msgraph"
)

type azKeyOps struct {
	applicationsClient *msgraph.ApplicationsClient
}

func New(applicationsClient *msgraph.ApplicationsClient) keyops.KeyOps {
	return &azKeyOps{applicationsClient: applicationsClient}
}

func (a *azKeyOps) Create(tenantID string, applicationID string) (keyops.Key, []byte, error) {
	createKeyRequest := msgraph.PasswordCredential{
		DisplayName: &applicationID,
	}

	// Set a 30 second timeout for the request
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	// Ensure that the context is canceled to prevent leaking resources
	defer cancel()

	logs.Info.Printf("creating new client secret for application with id %s...", applicationID)
	createdKey, statusCode, err := a.applicationsClient.AddPassword(ctx, applicationID, createKeyRequest)
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
	applicationData, statusCode, err := a.applicationsClient.Get(context.TODO(), key.Identifier, odata.Query{})
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

	logs.Info.Printf("deleting client secret: %s for application with id %s in tenant %s", key.ID, key.Identifier, key.Scope)
	statusCode, err := a.applicationsClient.RemovePassword(context.TODO(), key.Identifier, key.ID)
	if err != nil {
		return fmt.Errorf("error %d deleting client secret %s for application with id %s in tenant %s: %v", statusCode, key.ID, key.Identifier, key.Scope, err)
	}

	return nil
}
