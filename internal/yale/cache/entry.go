package cache

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// only lower alphanumeric, ., and - are legal in the names of k8s resources
var illegalK8sNameCharsRegexp = regexp.MustCompile(`[^a-z0-9.\-]`)

type Identifier interface {
	Identify() string
	// Scope is the hierarchiical containing resource within which a service account or client secret exists
	// For GCP this is the project, for Azure this is the tenant
	Scope() string
	Type() EntryType
	cacheSecretName() string
}

type GcpSaKeyEntryIdentifier struct {
	Email   string
	Project string
}

func (gcpIdentifier GcpSaKeyEntryIdentifier) Identify() string {
	return gcpIdentifier.Email
}

func (GcpSaKeyEntryIdentifier) Type() EntryType {
	return GcpSaKey
}

func (gcpIdentifier GcpSaKeyEntryIdentifier) Scope() string {
	return gcpIdentifier.Project
}

func (gcpIdentifier GcpSaKeyEntryIdentifier) cacheSecretName() string {
	// replace any characters that are illegal in kubernetes resource names (eg. "@") with "-"
	normalized := illegalK8sNameCharsRegexp.ReplaceAllString(gcpIdentifier.Email, "-")
	// replace anything that's not alphanumeric or . or - with -
	return secretNamePrefix + normalized
}

type AzureClientSecretEntryIdentifier struct {
	ApplicationID string
	TenantID      string
}

func (azureIdentifier AzureClientSecretEntryIdentifier) Identify() string {
	return azureIdentifier.ApplicationID
}

func (azureIdentifier AzureClientSecretEntryIdentifier) Scope() string {
	return azureIdentifier.TenantID
}

func (AzureClientSecretEntryIdentifier) Type() EntryType {
	return AzureClientSecret
}

func (azureIdentifier AzureClientSecretEntryIdentifier) cacheSecretName() string {
	// replace any characters that are illegal in kubernetes resource names (eg. "@") with "-"
	normalized := illegalK8sNameCharsRegexp.ReplaceAllString(azureIdentifier.ApplicationID, "-")
	// replace anything that's not alphanumeric or . or - with -
	return secretNamePrefix + normalized
}

// EntryIdentifier identifying information for a service account
type EntryIdentifier struct {
	Email         string    // Email for the service account
	Project       string    // Project the service account belongs to
	ApplicationID string    // Application ID for the Azure client secret
	TenantID      string    // Tenant ID for the Azure client secrets
	Type          EntryType // Type of the cache entry either GCPSAKey or AzureClientSecret are supported
}

// cacheSecretName return the name of the K8s secret that will be used to store the cache entry
func (sa EntryIdentifier) cacheSecretName() string {
	// replace any characters that are illegal in kubernetes resource names (eg. "@") with "-"
	normalized := illegalK8sNameCharsRegexp.ReplaceAllString(sa.Email, "-")
	// replace anything that's not alphanumeric or . or - with -
	return secretNamePrefix + normalized
}

// LastError information relating to the last error that occurred while processing this cache entry/service account
type LastError struct {
	// Message is the last error message
	Message string
	// Timestamp is the timestamp at which the last error occurred
	Timestamp time.Time
	// LastNotificationAt is the timestamp at which the last error notification was sent for this cache entry
	LastNotificationAt time.Time
}

// CurrentKey represents the current/active service account key that will
// be replicated to k8s secrets and Vault
type CurrentKey struct {
	// JSON representation of the service account key
	JSON string
	// ID service account key id
	ID string
	// CreatedAt time at which the  service account key was created
	CreatedAt time.Time
}

func newCacheEntry[I Identifier](identifier I) *Entry {
	return &Entry{
		Identifier:   identifier,
		RotatedKeys:  make(map[string]time.Time),
		DisabledKeys: make(map[string]time.Time),
		SyncStatus:   make(map[string]string),
	}
}

type EntryType int

const (
	GcpSaKey EntryType = iota + 1
	AzureClientSecret
)

type Entry struct {
	// EntryIdentifier identifying information for the service account the key belongs to
	Identifier
	// CurrentKey represents the current/active service account key that will
	// be replicated to k8s secrets and Vault
	CurrentKey CurrentKey
	// RotatedKeys map key id -> timestamp representing older versions of the key that were replaced
	// and should be disabled after a configured amount of time has passed
	RotatedKeys map[string]time.Time
	// DisabledKeys map key id -> timestamp representing older versions of the key that were disabled
	// and should be deleted after a configured amount of time has passed
	DisabledKeys map[string]time.Time
	// SyncStatus map used to track sync status for the GcpSaKey resources that use this cache entry.
	// Each entry in the map describes the last successful sync for a single GcpSaKey resource.
	// The entry's key is the name of the GcpSaKey, in the form "<namespace>/<name>".
	// The entry's value contains:
	// * the checksum of the GcpSaKey's JSON-marshalled spec
	// * the id of the key that was synced
	// concatenated with a ":", in the form "<checksum>:<key id>".
	//
	// Yale determines if it needs to perform a sync for a particular GcpSaKey by computing this value at runtime.
	// If the computed value does not match the stored value, it performs a key sync and updates the stored value.
	//
	// The advantages of this behavior are:
	// * If Yale fails to sync a value to Vault due to, say, a permissions issue, it will return an error
	//   and keep re-trying on every run until the sync succeeds
	// * If a sync succeeds, Yale will not attempt to sync again until the GcpSaKey's spec changes (say, for example,
	//   if the key needs to be synced to a different path) or the key is rotated. This avoids overwhelming Vault
	//   (or eventually Google secrets manager) with write requests.
	SyncStatus map[string]string
	// LastError information about the most recent error to occur while processing this cache entry
	LastError LastError
}

func (c *Entry) marshalToSecret(s *corev1.Secret) error {
	content, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("error marshalling Entry to JSON: %v", err)
	}
	name := c.Identifier.cacheSecretName()
	if s.Name == "" {
		s.Name = name
	} else if s.Name != name {
		return fmt.Errorf("error writing Entry to secret - expected name %q, has %q", name, s.Name)
	}
	if s.Labels == nil {
		s.Labels = make(map[string]string)
	}
	s.Labels[labelKey] = labelValue
	if s.Data == nil {
		s.Data = make(map[string][]byte)
	}
	s.Data[secretKey] = content
	return nil
}

func (c *Entry) unmarshalFromSecret(s *corev1.Secret) error {
	data, exists := s.Data[secretKey]
	if !exists {
		return fmt.Errorf("failed to unmarshal Entry from secret %s (missing %q key)", s.Name, secretKey)
	}
	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to unmarshal Entry from secret %s: %v", s.Name, err)
	}
	if c.RotatedKeys == nil {
		c.RotatedKeys = make(map[string]time.Time)
	}
	if c.DisabledKeys == nil {
		c.DisabledKeys = make(map[string]time.Time)
	}
	if c.SyncStatus == nil {
		c.SyncStatus = make(map[string]string)
	}
	return nil
}
