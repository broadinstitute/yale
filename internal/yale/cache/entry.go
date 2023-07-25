package cache

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/broadinstitute/yale/internal/yale/logs"
	corev1 "k8s.io/api/core/v1"
)

// only lower alphanumeric, ., and - are legal in the names of k8s resources
var illegalK8sNameCharsRegexp = regexp.MustCompile(`[^a-z0-9.\-]`)

// Identifier is an interface that can be implemented by any type that can be used to uniquely identify a cache entry
type Identifier interface {
	// Identify is the unique identifier for a yale managed resource. They serve as look up keys in the bundle map
	// examples are google service account email or azure application id
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
		Type:         identifier.Type(),
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

func (e EntryType) String() string {
	switch e {
	case GcpSaKey:
		return "GcpSaKey"
	case AzureClientSecret:
		return "AzureClientSecret"
	}
	return fmt.Sprintf("unknown entry type: %d", e)
}

type Entry struct {
	// EntryIdentifier identifying information for the service account the key belongs to
	Identifier
	// Type the resource type of the cache entry
	// either GCPSAKey or AzureClientSecret are supported
	Type EntryType
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

// UnmarshalJSON custom unmarshaling logic to account the fact that the data stored in the cache may have a different shape based on
// the entry type. This ensures that cache secrets can be unmarshaled into the correct concrete type of either GCPSAKey or AzureClientSecret identifiers
// TODO: is there a better way to do this?
func (e *Entry) UnmarshalJSON(data []byte) error {
	// first we need to extract the entry type from the JSON to determine which concrete struct type to unmarshal into
	entryData := make(map[string]interface{})
	err := json.Unmarshal(data, &entryData)
	if err != nil {
		return fmt.Errorf("error unmarshaling Entry JSON: %v", err)
	}

	_, exists := entryData["Type"]
	if !exists {
		logs.Info.Print("unmarshaling legacy cache entry")
		if err := e.handleUnmarshalLegacyCacheEntry(entryData); err != nil {
			return err
		}
	} else {
		entryType, ok := entryData["Type"].(float64)
		if !ok {
			return fmt.Errorf("error unmarshaling Entry JSON: Type is not a number")
		}
		e.Type = EntryType(entryType)

		// extract the identifier data
		identifierData, err := json.Marshal(entryData["Identifier"])
		if err != nil {
			return fmt.Errorf("error unmarshaling Entry JSON: %v", err)
		}
		switch e.Type {
		case GcpSaKey:
			var identifier GcpSaKeyEntryIdentifier
			err = json.Unmarshal(identifierData, &identifier)
			if err != nil {
				return fmt.Errorf("error unmarshaling GcpSaKeyEntryIdentifier: Identifier is not a GcpSaKeyEntryIdentifier")
			}
			e.Identifier = identifier
		case AzureClientSecret:
			var identifier AzureClientSecretEntryIdentifier
			err = json.Unmarshal(identifierData, &identifier)
			if err != nil {
				return fmt.Errorf("error unmarshaling AzureClientSecretEntryIdentifier: Identifier is not a AzureClientSecretEntryIdentifier")
			}
			e.Identifier = identifier
		default:
			return fmt.Errorf("unsupported Entry type: %v", e.Type)
		}
	}

	currentKeyData, err := json.Marshal(entryData["CurrentKey"])
	if err != nil {
		return fmt.Errorf("error parsing current key data: %v", err)
	}
	var currentKey CurrentKey
	err = json.Unmarshal(currentKeyData, &currentKey)
	if err != nil {
		return fmt.Errorf("error unmarshaling CurrentKey: CurrentKey is not a CurrentKey")
	}
	e.CurrentKey = currentKey

	rotatedKeysData, err := json.Marshal(entryData["RotatedKeys"])
	if err != nil {
		return fmt.Errorf("error parsing rotated keys data: %v", err)
	}
	rotatedKeys := make(map[string]time.Time)
	err = json.Unmarshal(rotatedKeysData, &rotatedKeys)
	if err != nil {
		return fmt.Errorf("error unmarshaling RotatedKeys: RotatedKeys is not a map[string]time.Time")
	}
	e.RotatedKeys = rotatedKeys

	disabledKeysData, err := json.Marshal(entryData["DisabledKeys"])
	if err != nil {
		return fmt.Errorf("error parsing disabled keys data: %v", err)
	}
	disabledKeys := make(map[string]time.Time)
	err = json.Unmarshal(disabledKeysData, &disabledKeys)
	if err != nil {
		return fmt.Errorf("error unmarshaling DisabledKeys: DisabledKeys is not a map[string]time.Time")
	}
	e.DisabledKeys = disabledKeys

	syncStatusData, err := json.Marshal(entryData["SyncStatus"])
	if err != nil {
		return fmt.Errorf("error parsing sync status data: %v", err)
	}
	syncStatus := make(map[string]string)
	err = json.Unmarshal(syncStatusData, &syncStatus)
	if err != nil {
		return fmt.Errorf("error unmarshaling SyncStatus: SyncStatus is not a map[string]string")
	}
	e.SyncStatus = syncStatus

	lastErrorData, err := json.Marshal(entryData["LastError"])
	if err != nil {
		return fmt.Errorf("error parsing last error data: %v", err)
	}
	var lastError LastError
	err = json.Unmarshal(lastErrorData, &lastError)
	if err != nil {
		return fmt.Errorf("error unmarshaling LastError: LastError is not a LastError")
	}
	e.LastError = lastError

	return nil
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

func (e *Entry) handleUnmarshalLegacyCacheEntry(entryData map[string]interface{}) error {
	// legacy cache entries will not have the Type field set, so we need to set it here
	// legacy cache entries are guaranteed to be GcpSaKey entries
	e.Type = GcpSaKey

	// rather than having an identifier field, legacy cache entries will instead have a ServiceServiceAccount field
	// unmarshal the service account data into a GcpSaKeyEntryIdentifier
	_, exists := entryData["ServiceAccount"]
	if !exists {
		return fmt.Errorf("error unmarshaling legacy cache entry: missing ServiceAccount field")
	}

	serviceAccountData, err := json.Marshal(entryData["ServiceAccount"])
	if err != nil {
		return fmt.Errorf("error parsing service account data: %v", err)
	}
	var identifier GcpSaKeyEntryIdentifier
	err = json.Unmarshal(serviceAccountData, &identifier)
	if err != nil {
		return fmt.Errorf("error unmarshaling GcpSaKeyEntryIdentifier: ServiceAccount is not a GcpSaKeyEntryIdentifier")
	}
	e.Identifier = identifier
	return nil
}
