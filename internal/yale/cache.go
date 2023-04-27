package yale

import (
	"context"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/cache"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"google.golang.org/api/iam/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"time"
)

// annotations added by Yale to its target secret
const validAfterAnnotation = "validAfterDate"
const currentServiceAccountKeyNameAnnotation = "serviceAccountKeyName"
const oldServiceAccountKeyNameAnnotation = "oldServiceAccountKeyName"

const timeInADay = time.Hour * 24

func (m *Yale) PopulateCache() error {
	logs.Info.Printf("Syncing Yale cache (namespace: %s)...", m.options.CacheNamespace)

	if err := m.deleteAllCacheEntries(); err != nil {
		return err
	}
	keysMap, err := m.getKeysMap()
	if err != nil {
		return err
	}

	for _, gcpSaKey := range keysMap {
		secret, err := m.k8s.CoreV1().Secrets(gcpSaKey.Namespace).Get(context.Background(), gcpSaKey.Spec.Secret.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("error retrieving secret for GCPSaKey %s/%s: %v", gcpSaKey.Namespace, gcpSaKey.Name, err)
		}

		if secret.Annotations[validAfterAnnotation] == "" {
			return fmt.Errorf("error reading secret %s for GCPSaKey %s/%s: missing annotation %q", secret.Name, gcpSaKey.Namespace, gcpSaKey.Name, validAfterAnnotation)
		}
		createdAt, err := time.Parse(time.RFC3339, secret.Annotations[validAfterAnnotation])
		if err != nil {
			return fmt.Errorf("error reading secret %s for GCPSaKey %s/%s: error parsing %s annotation: %v", secret.Name, gcpSaKey.Namespace, gcpSaKey.Name, validAfterAnnotation, err)
		}

		if secret.Annotations[currentServiceAccountKeyNameAnnotation] == "" {
			return fmt.Errorf("error reading secret %s for GCPSaKey %s/%s: missing annotation %q", secret.Name, gcpSaKey.Namespace, gcpSaKey.Name, currentServiceAccountKeyNameAnnotation)
		}
		currentKeyId := extractServiceAccountKeyIdFromFullName(secret.Annotations[currentServiceAccountKeyNameAnnotation])

		jsonKeyData, exists := secret.Data[gcpSaKey.Spec.Secret.JsonKeyName]
		if !exists || len(jsonKeyData) == 0 {
			return fmt.Errorf("error reading secret %s for GCPSaKey %s/%s: json-formatted key is missing from secret", secret.Name, gcpSaKey.Namespace, gcpSaKey.Name)
		}

		var cacheEntry cache.Entry
		cacheEntry.ServiceAccount.Email = gcpSaKey.Spec.GoogleServiceAccount.Name
		cacheEntry.ServiceAccount.Project = gcpSaKey.Spec.GoogleServiceAccount.Project

		cacheEntry.CurrentKey.ID = currentKeyId
		cacheEntry.CurrentKey.CreatedAt = createdAt
		cacheEntry.CurrentKey.JSON = string(jsonKeyData)

		cacheEntry.RotatedKeys = make(map[string]time.Time)
		cacheEntry.DisabledKeys = make(map[string]time.Time)

		oldKeyName := secret.Annotations[oldServiceAccountKeyNameAnnotation]
		if oldKeyName != "" {
			if err := m.addRotatedKeyToCacheEntry(&cacheEntry, gcpSaKey, extractServiceAccountKeyIdFromFullName(oldKeyName)); err != nil {
				return err
			}
		}

		if err := m.writeCacheEntryToNewSecret(cacheEntry); err != nil {
			return fmt.Errorf("error writing cache entry for %s to new secret: %v", cacheEntry.ServiceAccount.Email, err)
		}
	}
	return nil
}

func (m *Yale) writeCacheEntryToNewSecret(entry cache.Entry) error {
	var s corev1.Secret
	if err := entry.MarshalToSecret(&s); err != nil {
		return fmt.Errorf("error marshalling cache entry for %s to secret", entry.ServiceAccount.Email)
	}
	logs.Info.Printf("Writing cache entry for %s to new secret %s", entry.ServiceAccount.Email, s.Name)
	_, err := m.k8s.CoreV1().Secrets(m.options.CacheNamespace).Create(context.Background(), &s, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("error writing new cache entry secret %s for %s: %v", s.Name, entry.ServiceAccount.Email, err)
	}
	return nil
}

// return a map of GCPSaKeys keyed by service account email, returning an error if
// multiple GCPSaKeys exist for the same service account email
func (m *Yale) getKeysMap() (map[string]v1beta1.GCPSaKey, error) {
	gcpSaKeyList, err := m.GetGCPSaKeyList()
	if err != nil {
		return nil, fmt.Errorf("error retrieving list of service account keys: %v", err)
	}

	var multiplesErr error
	result := make(map[string]v1beta1.GCPSaKey)
	for _, gcpSaKey := range gcpSaKeyList.Items {
		email := gcpSaKey.Spec.GoogleServiceAccount.Name
		match, exists := result[email]

		if exists {
			multiplesErr = fmt.Errorf("found multiple GCPSaKey resources for service account %s: %s/%s, %s/%s", email, gcpSaKey.Namespace, gcpSaKey.Name, match.Namespace, match.Name)
			logs.Warn.Printf(multiplesErr.Error())
			continue
		}
		result[email] = gcpSaKey
	}

	if multiplesErr != nil {
		return nil, multiplesErr
	}
	return result, nil
}

// add the old rotated key, if there is one, to the appropriate field of the cache entry
func (m *Yale) addRotatedKeyToCacheEntry(entry *cache.Entry, gcpSaKey v1beta1.GCPSaKey, rotatedKeyId string) error {
	metadata, err := m.getSaKey(entry.ServiceAccount.Project, entry.ServiceAccount.Email, rotatedKeyId)
	if err != nil {
		return fmt.Errorf("error retrieving metadata for sa key id %s (%s) from gcp: %v", rotatedKeyId, entry.ServiceAccount.Email, err)
	}
	// old key would have been rotated at the same time the current key was created
	rotatedAt := entry.CurrentKey.CreatedAt

	// key was not disabled, just rotated
	if !metadata.Disabled {
		entry.RotatedKeys[rotatedKeyId] = rotatedAt
		return nil
	}

	// key was disabled. so approximate the "disabledAt" timestamp by adding
	// the GCPSaKey's DisableAfter duration to the rotatedAt timestamp
	disableBuffer := timeInADay * time.Duration(gcpSaKey.Spec.KeyRotation.DisableAfter)
	disabledAt := rotatedAt.Add(disableBuffer)
	entry.DisabledKeys[rotatedKeyId] = disabledAt
	return nil
}

// fetch service account key metadata from Google Cloud IAM API
func (m *Yale) getSaKey(project string, serviceAccountEmail string, keyId string) (*iam.ServiceAccountKey, error) {
	requestPath := fmt.Sprintf("projects/%s/serviceAccounts/%s/keys/%s", project, serviceAccountEmail, keyId)
	return m.gcp.Projects.ServiceAccounts.Keys.Get(requestPath).Context(context.Background()).Do()
}

func (m *Yale) deleteAllCacheEntries() error {
	logs.Info.Printf("Deleting all cache entries in namespace %s (selector: %q)", m.options.CacheNamespace, cache.Selector())
	return m.k8s.CoreV1().Secrets(m.options.CacheNamespace).DeleteCollection(context.Background(), metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: cache.Selector(),
	})
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
