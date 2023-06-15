package keyops

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/broadinstitute/yale/internal/yale/logs"
	"google.golang.org/api/iam/v1"
)

// keyAlgorithm what key algorithm to use when creating new Google SA keys
const keyAlgorithm string = "KEY_ALG_RSA_2048"

// keyFormat format to use when creating new Google SA keys
const keyFormat string = "TYPE_GOOGLE_CREDENTIALS_FILE"

// Key represents a Google IAM service account key
type Key struct {
	// Project name of the project that owns the service account the key belongs to
	Project string
	// ServiceAccountEmail email for the service account that owns the key
	ServiceAccountEmail string
	// ID alphanumeric ID for the key
	ID string
}

// KeyOps peforms operations on Google service account keys. It supports
// creating new keys, disabling, and deleting them.
type KeyOps interface {
	// Create a new service account key for the given service account
	// returns a Key instance that includes the new key's ID as well as the key's JSON private key data
	Create(project string, serviceAccountEmail string) (Key, []byte, error)
	// IsDisabled return true if the given key is enabled, false otherwise
	IsDisabled(key Key) (bool, error)
	// EnsureDisabled check if the key is enabled and if so, disable it
	EnsureDisabled(key Key) error
	// DeleteIfDisabled if the service account key is disabled, delete it, else return an error
	DeleteIfDisabled(key Key) error
}

func New(iamService *iam.Service) KeyOps {
	return &keyops{
		iam: iamService,
	}
}

type keyops struct {
	iam *iam.Service
}

func (k *keyops) Create(project string, serviceAccountEmail string) (Key, []byte, error) {
	name := qualifiedServiceAccountName(project, serviceAccountEmail)
	ctx := context.Background()
	request := &iam.CreateServiceAccountKeyRequest{
		KeyAlgorithm:   keyAlgorithm,
		PrivateKeyType: keyFormat,
	}

	logs.Info.Printf("creating new service account for %s...", serviceAccountEmail)
	newKey, err := k.iam.Projects.ServiceAccounts.Keys.Create(name, request).Context(ctx).Do()
	if err != nil {
		return Key{}, nil, fmt.Errorf("error creating new service account key for %s: %v", name, err)
	}

	keyID := extractServiceAccountKeyIdFromFullName(newKey.Name)
	logs.Info.Printf("created new service account key %s for %s", keyID, serviceAccountEmail)

	jsonData, err := base64.StdEncoding.DecodeString(newKey.PrivateKeyData)
	if err != nil {
		return Key{}, nil, fmt.Errorf("error decoding private key data for new service account key %s for %s: %v", keyID, serviceAccountEmail, err)
	}

	return Key{
		Project:             project,
		ServiceAccountEmail: serviceAccountEmail,
		ID:                  keyID,
	}, jsonData, nil
}

func (k *keyops) IsDisabled(key Key) (bool, error) {
	resp, err := k.iam.Projects.ServiceAccounts.Keys.Get(key.qualifiedKeyName()).Context(context.Background()).Do()
	if err != nil {
		return false, fmt.Errorf("api request for %s failed: %v", key.qualifiedKeyName(), err)
	}

	return resp.Disabled, nil
}

func (k *keyops) EnsureDisabled(key Key) error {
	disabled, err := k.IsDisabled(key)
	if err != nil {
		return err
	}
	if disabled {
		logs.Info.Printf("won't disable %s; key is already disabled", key.qualifiedKeyName())
		return nil
	}

	logs.Info.Printf("disabling %s", key.qualifiedKeyName())
	request := &iam.DisableServiceAccountKeyRequest{}
	_, err = k.iam.Projects.ServiceAccounts.Keys.Disable(key.qualifiedKeyName(), request).Context(context.Background()).Do()
	if err != nil {
		return fmt.Errorf("api request to disable %s failed: %v", key.qualifiedKeyName(), err)
	}
	return nil
}

func (k *keyops) DeleteIfDisabled(key Key) error {
	disabled, err := k.IsDisabled(key)
	if err != nil {
		return err
	}

	if !disabled {
		return fmt.Errorf("key %s is not disabled; please manually verify it is not in use and disable it", key.qualifiedKeyName())
	}

	logs.Info.Printf("deleting %s", key.qualifiedKeyName())
	_, err = k.iam.Projects.ServiceAccounts.Keys.Delete(key.qualifiedKeyName()).Context(context.Background()).Do()
	return err
}

// return qualified key name for use in IAM api calls.
// eg. "projects/my-project/serviceAccounts/my-service-account@my-project/keys/123"
func (k Key) qualifiedKeyName() string {
	return qualifiedKeyName(k.Project, k.ServiceAccountEmail, k.ID)
}

// return qualified key name for use in IAM api calls.
// eg. "projects/my-project/serviceAccounts/my-service-account@my-project/keys/123"
func qualifiedKeyName(project string, email string, keyID string) string {
	prefix := qualifiedServiceAccountName(project, email)
	return fmt.Sprintf("%s/keys/%s", prefix, keyID)
}

// return qualified service account name for use in IAM api calls.
// eg. "projects/my-project/serviceAccounts/my-service-account@my-project"
func qualifiedServiceAccountName(project string, email string) string {
	return fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)
}

// "projects/broad-dsde-qa/serviceAccounts/whoever@gserviceaccount.com/keys/abcdef0123"
// ->
// "abcdef0123"
func extractServiceAccountKeyIdFromFullName(name string) string {
	tokens := strings.SplitN(name, "/keys/", 2)
	if len(tokens) != 2 {
		return ""
	}
	return tokens[1]
}
