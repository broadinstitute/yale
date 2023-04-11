package cache

import (
	"encoding/json"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"regexp"
	"time"
)

// DefaultCacheNamespace default namespace where Yale should store cached service account keys
const DefaultCacheNamespace = "yale-cache"

// label key/value pair added to all cache entry Secrets
const labelKey = "yale.terra.bio/cache-entry"
const labelValue = "true"

// key within the secret where marshaled cache entry data is stored
const secretKey = "value"

// prefix for cache entry secret names
const secretNamePrefix = "yale-cache-"

type Entry struct {
	// ServiceAccount identifying information for the service account the key belongs to
	ServiceAccount struct {
		Email   string // Email for the service account
		Project string // Project the service account belongs to
	}
	// CurrentKey represents the current/active service account key that will
	// be replicated to k8s secrets and Vault
	CurrentKey struct {
		// JSON representation of the service account key
		JSON string
		// ID service account key id
		ID string
		// CreatedAt time at which the  service account key was created
		CreatedAt time.Time
	}
	// RotatedKeys map key id -> timestamp representing older versions of the key that were replaced
	// and should be disabled after a configured amount of time has passed
	RotatedKeys map[string]time.Time
	// DisabledKeys map key id -> timestamp representing older versions of the key that were disabled
	// and should be deleted after a configured amount of time has passed
	DisabledKeys map[string]time.Time
}

// only lower alphanumeric, ., and - are legal in the names of k8s resources
var illegalK8sNameCharsRegexp = regexp.MustCompile(`[^a-z0-9.\-]`)

func SecretName(serviceAccountEmail string) string {
	// replace any characters that are illegal in kubernetes resource names (eg. "@") with "-"
	normalized := illegalK8sNameCharsRegexp.ReplaceAllString(serviceAccountEmail, "-")
	// replace anything that's not alphanumeric or . or - with -
	return secretNamePrefix + normalized
}

// Selector returns a label selector that will match all CacheEntries in a namespace
func Selector() string {
	return labelKey + "=" + labelValue
}

func (c *Entry) MarshalToSecret(s *corev1.Secret) error {
	content, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("error marshalling Entry to JSON: %v", err)
	}
	name := SecretName(c.ServiceAccount.Email)
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

func (c *Entry) UnmarshalFromSecret(s *corev1.Secret) error {
	data, exists := s.Data[secretKey]
	if !exists {
		return fmt.Errorf("failed to unmarshal Entry from secret %s (missing %q key)", s.Name, secretKey)
	}
	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to unmarshal Entry from secret %s: %v", s.Name, err)
	}
	return nil
}