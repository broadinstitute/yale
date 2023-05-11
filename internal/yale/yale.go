package yale

import (
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/authmetrics"
	"github.com/broadinstitute/yale/internal/yale/cache"
	"github.com/broadinstitute/yale/internal/yale/client"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/cutoff"
	"github.com/broadinstitute/yale/internal/yale/keyops"
	"github.com/broadinstitute/yale/internal/yale/keysync"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"github.com/broadinstitute/yale/internal/yale/resourcemap"
	"time"
)

type Yale struct { // Yale config
	options     Options
	authmetrics authmetrics.AuthMetrics
	keyops      keyops.Keyops
	keysync     keysync.KeySync
	cache       cache.Cache
	resourcemap resourcemap.Mapper
}

type Options struct {
	CacheNamespace string
}

// NewYale /* Construct a new Yale Manager */
func NewYale(clients *client.Clients, opts ...func(*Options)) (*Yale, error) {
	options := Options{
		CacheNamespace: cache.DefaultCacheNamespace,
	}
	for _, opt := range opts {
		opt(&options)
	}

	k8s := clients.GetK8s()
	iam := clients.GetGCP()
	crd := clients.GetCRDs()
	_authmetrics := authmetrics.New(clients.GetMetrics(), iam)
	_keyops := keyops.New(iam)
	_keysync := keysync.New(k8s, clients.GetVault())
	_cache := cache.New(k8s, options.CacheNamespace)
	_resourcemap := resourcemap.New(crd, _cache)

	return &Yale{options, _authmetrics, _keyops, _keysync, _cache, _resourcemap}, nil
}

func (m *Yale) Run() error {
	resources, err := m.resourcemap.Build()
	if err != nil {
		return fmt.Errorf("error inspecting cluster for cache entries and GcpSaKey resources: %v", err)
	}

	for _, bundle := range resources {
		if err = m.processServiceAccount(bundle.Entry, bundle.GSKs); err != nil {
			return err
		}
	}
	return nil
}

func (m *Yale) processServiceAccount(entry *cache.Entry, gsks []apiv1b1.GCPSaKey) error {
	var err error

	cutoffs := m.computeCutoffs(entry, gsks)

	if err = m.rotateKey(entry, cutoffs, gsks); err != nil {
		return err
	}
	if err = m.disableOldKeys(entry, cutoffs); err != nil {
		return err
	}
	if err = m.deleteOldKeys(entry, cutoffs); err != nil {
		return err
	}
	if err = m.retireCacheEntryIfNeeded(entry, gsks); err != nil {
		return err
	}

	return nil
}

// computeCutoffs computes the cutoffs for key rotation/disabling/deletion based on the GcpSaKey resources
// for this service account
func (m *Yale) computeCutoffs(entry *cache.Entry, gsks []apiv1b1.GCPSaKey) cutoff.Cutoffs {
	if len(gsks) == 0 {
		logs.Info.Printf("cache entry for %s has no corresponding GcpSaKey resources in the cluster; will use Yale's default cutoffs to retire old keys", entry.ServiceAccount.Email)
		return cutoff.NewWithDefaults()
	}
	return cutoff.New(gsks...)
}

// rotateKey rotates the current active key for the service account, if needed.
func (m *Yale) rotateKey(entry *cache.Entry, cutoffs cutoff.Cutoffs, gsks []apiv1b1.GCPSaKey) error {
	var err error

	if err = m.issueNewKeyIfNeeded(entry, cutoffs, gsks); err != nil {
		return err
	}
	if err = m.keysync.SyncIfNeeded(entry, gsks...); err != nil {
		return err
	}
	if err = m.cache.Save(entry); err != nil {
		return err
	}

	return nil
}

// issueNewKeyIfNeed given cache entry and gsk, checks if the entry's current active key needs to be rotated.
// if a rotation is needed (or the cache entry is new/empty), it issues a new sa key, adds it
// to the cache entry, then saves the updated cache entry to k8s.
func (m *Yale) issueNewKeyIfNeeded(entry *cache.Entry, cutoffs cutoff.Cutoffs, gsks []apiv1b1.GCPSaKey) error {
	email := entry.ServiceAccount.Email
	project := entry.ServiceAccount.Project

	if entry.CurrentKey.ID != "" {
		logs.Info.Printf("service account %s: checking if current key %s needs rotation (created at %s; rotation age is %d days)", email, entry.CurrentKey.ID, entry.CurrentKey.CreatedAt, cutoffs.RotateAfterDays())

		if cutoffs.ShouldRotate(entry.CurrentKey.CreatedAt) {
			logs.Info.Printf("service account %s: rotating key %s", email, entry.CurrentKey.ID)
			entry.RotatedKeys[entry.CurrentKey.ID] = time.Now()
			entry.CurrentKey = cache.CurrentKey{}
		} else {
			logs.Info.Printf("service account %s: current key %s does not need rotation", email, entry.CurrentKey.ID)
		}
	}

	if entry.CurrentKey.ID == "" {
		if len(gsks) == 0 {
			logs.Info.Printf("service account %s: no remaining GcpSaKeys for this service account in the cluster, won't issue new key", email)
		} else {
			logs.Info.Printf("service account %s: issuing new key", email)
			newKey, json, err := m.keyops.Create(project, email)
			if err != nil {
				return fmt.Errorf("error issuing new service account key for %s: %v", email, err)
			}

			logs.Info.Printf("created new service account key %s for %s", newKey.ID, email)

			entry.CurrentKey.ID = newKey.ID
			entry.CurrentKey.JSON = string(json)
			entry.CurrentKey.CreatedAt = time.Now()
		}
	}

	if err := m.cache.Save(entry); err != nil {
		return fmt.Errorf("error saving cache entry for %s after key rotation: %v", email, err)
	}

	return nil
}

func (m *Yale) disableOldKeys(entry *cache.Entry, cutoffs cutoff.Cutoffs) error {
	for keyId, rotatedAt := range entry.DisabledKeys {
		if err := m.disableOneKey(keyId, rotatedAt, entry, cutoffs); err != nil {
			return err
		}
	}
	return nil
}

func (m *Yale) disableOneKey(keyId string, rotatedAt time.Time, entry *cache.Entry, cutoffs cutoff.Cutoffs) error {
	// has enough time passed since rotation? if not, do nothing
	logs.Info.Printf("key %s (service account %s) was rotated at %s, disable cutoff is %d days", keyId, entry.ServiceAccount.Email, rotatedAt, cutoffs.DisableAfterDays())
	if !cutoffs.ShouldDisable(rotatedAt) {
		logs.Info.Printf("key %s (service account %s): too early to disable", keyId, entry.ServiceAccount.Email)
		return nil
	}

	// check if the key is still in use
	logs.Info.Printf("key %s (service account %s) has reached disable cutoff; checking if still in use", keyId, entry.ServiceAccount.Email)
	lastAuthTime, err := m.authmetrics.LastAuthTime(entry.ServiceAccount.Project, entry.ServiceAccount.Email, keyId)
	if err != nil {
		return fmt.Errorf("error determining last authentication time for key %s (service account %s): %v", keyId, entry.ServiceAccount.Email, err)
	}
	if lastAuthTime == nil {
		logs.Info.Printf("could not identify last authentication time for key %s (service account %s); assuming key is not in use", keyId, entry.ServiceAccount.Email)
	} else {
		logs.Info.Printf("last authentication time for key %s (service account %s): %s", keyId, entry.ServiceAccount.Email, *lastAuthTime)
		if !cutoffs.SafeToDisable(*lastAuthTime) {
			return fmt.Errorf("key %s (service account %s) was rotated at %s but was last used to authenticate at %s; please find out what's still using this key and fix it", keyId, entry.ServiceAccount.Email, rotatedAt, *lastAuthTime)
		}
	}

	// disable the key
	logs.Info.Printf("disabling key %s (service account %s)...", keyId, entry.ServiceAccount.Email)
	if err = m.keyops.EnsureDisabled(keyops.Key{
		Project:             entry.ServiceAccount.Project,
		ServiceAccountEmail: entry.ServiceAccount.Email,
		ID:                  keyId,
	}); err != nil {
		return fmt.Errorf("error disabling key %s (service account %s): %v", keyId, entry.ServiceAccount.Email, err)
	}

	// update cache entry to reflect that the key was successfully disabled
	delete(entry.RotatedKeys, keyId)
	entry.DisabledKeys[keyId] = time.Now()
	if err = m.cache.Save(entry); err != nil {
		return fmt.Errorf("error saving cache entry after key disable: %v", err)
	}
	return nil
}

// deleteOldKeys will delete old service account keys
func (m *Yale) deleteOldKeys(entry *cache.Entry, cutoffs cutoff.Cutoffs) error {
	for keyId, disabledAt := range entry.DisabledKeys {
		if err := m.deleteOneKey(keyId, disabledAt, entry, cutoffs); err != nil {
			return err
		}
	}
	return nil
}

func (m *Yale) deleteOneKey(keyId string, disabledAt time.Time, entry *cache.Entry, cutoffs cutoff.Cutoffs) error {
	// has enough time passed since this key was disabled? if not, do nothing
	logs.Info.Printf("key %s (service account %s) was disabled at %s, delete cutoff is %d days", keyId, entry.ServiceAccount.Email, disabledAt, cutoffs.DisableAfterDays())
	if !cutoffs.ShouldDelete(disabledAt) {
		logs.Info.Printf("key %s (service account %s): too early to delete", keyId, entry.ServiceAccount.Email)
		return nil
	}

	key := keyops.Key{
		Project:             entry.ServiceAccount.Project,
		ServiceAccountEmail: entry.ServiceAccount.Email,
		ID:                  keyId,
	}

	// delete key from GCP
	logs.Info.Printf("key %s (service account %s) has reached delete cutoff; deleting it", key.ID, key.ServiceAccountEmail)
	if err := m.keyops.DeleteIfDisabled(key); err != nil {
		return fmt.Errorf("error deleting key %s (service account %s): %v", keyId, entry.ServiceAccount.Email, err)
	}

	// delete key from cache entry
	delete(entry.DisabledKeys, keyId)
	if err := m.cache.Save(entry); err != nil {
		return fmt.Errorf("error updating cache entry for %s after key deletion: %v", entry.ServiceAccount.Email, err)
	}

	logs.Info.Printf("deleted key %s (service account %s)", key.ID, key.ServiceAccountEmail)

	return nil
}

func (m *Yale) retireCacheEntryIfNeeded(entry *cache.Entry, gsks []apiv1b1.GCPSaKey) error {
	if len(gsks) > 0 {
		return nil
	}
	if len(entry.CurrentKey.ID) > 0 {
		logs.Info.Printf("cache entry for %s has no corresponding GcpSaKey resources in the cluster; will not delete it because it still has a current key", entry.ServiceAccount.Email)
		return nil
	}
	if len(entry.RotatedKeys) > 0 {
		logs.Info.Printf("cache entry for %s has no corresponding GcpSaKey resources in the cluster; will not delete it because it still has keys to disable", entry.ServiceAccount.Email)
		return nil
	}
	if len(entry.DisabledKeys) > 0 {
		logs.Info.Printf("cache entry for %s has no corresponding GcpSaKey resources in the cluster; will not delete it because it still has keys to delete", entry.ServiceAccount.Email)
		return nil
	}

	logs.Info.Printf("cache entry for %s is empty and has no corresponding GcpSaKey resources in the cluster; deleting it", entry.ServiceAccount.Email)
	return m.cache.Delete(entry)
}
