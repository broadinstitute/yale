package cache

import (
	"context"
	"fmt"

	"github.com/broadinstitute/yale/internal/yale/logs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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

type Cache interface {
	// List returns all cache entries in the cache namespace
	List() ([]*Entry, error)
	// GetOrCreate will retrieve the cache entry for the given service account, or create a new empty
	// cache entry if one doesn't exist
	GetOrCreate(Identifier) (*Entry, error)
	// Save persists a cache entry to the cluster
	Save(*Entry) error
	// Delete deletes a cache entry from the cluster
	Delete(*Entry) error
}

func New(k8s kubernetes.Interface, namespace string) Cache {
	return &cache{
		namespace: namespace,
		k8s:       k8s,
	}
}

type cache struct {
	namespace string
	k8s       kubernetes.Interface
}

func (c *cache) List() ([]*Entry, error) {
	resp, err := c.k8s.CoreV1().Secrets(c.namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector(),
	})
	if err != nil {
		return nil, fmt.Errorf("error listing secrets in namespace %s: %v", c.namespace, err)
	}

	var entries []*Entry
	for _, secret := range resp.Items {
		entry := &Entry{}
		if err = entry.unmarshalFromSecret(&secret); err != nil {
			return nil, fmt.Errorf("error unmarshalling cache entry secret %s: %v", secret.Name, err)
		}
		if entry.Identify() == "" {
			return nil, fmt.Errorf("invalid cache entry secret %s: missing service account email", secret.Name)
		}
		if entry.Scope() == "" {
			return nil, fmt.Errorf("invalid cache entry secret %s: missing service account project", secret.Name)
		}
		if secret.Name != entry.cacheSecretName() {
			return nil, fmt.Errorf("invalid cache entry secret %s: secret name does not match service account, should be %s", secret.Name, entry.cacheSecretName())
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (c *cache) GetOrCreate(identifier Identifier) (*Entry, error) {
	secret, err := c.k8s.CoreV1().Secrets(c.namespace).Get(context.Background(), identifier.cacheSecretName(), metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("error checking for existing cache entry for service account %s: %v", identifier.Identify(), err)
		}

		logs.Info.Printf("secret %s does not exist in cache namespace %s, creating new cache entry for %s", identifier.cacheSecretName(), c.namespace, identifier.Identify())
		return c.createAndSaveNewEmptyCacheEntry(identifier)
	}

	var entry Entry
	err = (&entry).unmarshalFromSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling cache entry secret %s: %v", secret.Name, err)
	}
	return &entry, nil
}

func (c *cache) Save(entry *Entry) error {
	identifier := entry.Identify()
	secretName := entry.cacheSecretName()

	secret, err := c.k8s.CoreV1().Secrets(c.namespace).Get(context.Background(), secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error reading existing cache entry for %s: %v", identifier, err)
	}
	if err = entry.marshalToSecret(secret); err != nil {
		return fmt.Errorf("error marshalling cache entry for %s to secret: %v", identifier, err)
	}
	_, err = c.k8s.CoreV1().Secrets(c.namespace).Update(context.Background(), secret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("error updating existing cache entry for %s: %v", identifier, err)
	}
	return nil
}

func (c *cache) Delete(entry *Entry) error {
	if err := c.k8s.CoreV1().Secrets(c.namespace).Delete(context.Background(), entry.cacheSecretName(), metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("error deleting cache entry secret %s for %s: %v", entry.cacheSecretName(), entry.Identify(), err)
	}
	return nil
}

// create a new empty cache entry and save it to the cluster
func (c *cache) createAndSaveNewEmptyCacheEntry(identifier Identifier) (*Entry, error) {
	logs.Info.Printf("creating new cache entry for %s", identifier.Identify())
	entry := newCacheEntry(identifier)

	var secret corev1.Secret
	if err := entry.marshalToSecret(&secret); err != nil {
		return nil, fmt.Errorf("error marshalling cache entry for %s to secret: %v", identifier.Identify(), err)
	}
	logs.Info.Printf("saving new empty cache entry for %s to secret %s in %s", identifier.Identify(), secret.Name, c.namespace)
	_, err := c.k8s.CoreV1().Secrets(c.namespace).Create(context.Background(), &secret, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("error saving cache entry for %s to secret %s in %s: %v", identifier.Identify(), secret.Name, c.namespace, err)
	}

	return entry, nil
}

// labelSelector returns a label selector that will match all CacheEntries in a namespace
func labelSelector() string {
	return labelKey + "=" + labelValue
}
