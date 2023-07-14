package yale

import (
	"fmt"
	"strings"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/broadinstitute/yale/internal/yale/authmetrics"
	"github.com/broadinstitute/yale/internal/yale/cache"
	"github.com/broadinstitute/yale/internal/yale/client"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/crd/clientset/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/cutoff"
	"github.com/broadinstitute/yale/internal/yale/keyops"
	"github.com/broadinstitute/yale/internal/yale/keyops/azurekeyops"
	"github.com/broadinstitute/yale/internal/yale/keysync"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"github.com/broadinstitute/yale/internal/yale/resourcemap"
	"github.com/broadinstitute/yale/internal/yale/slack"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/manicminer/hamilton/msgraph"
	"google.golang.org/api/iam/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	gcpKeyops   = "gcp"
	azureKeyops = "azure"
)

type Yale struct { // Yale config
	options     Options
	cache       cache.Cache
	resourcemap resourcemap.Mapper
	keyops      map[string]keyops.KeyOps
	keysync     keysync.KeySync
	authmetrics authmetrics.AuthMetrics
	slack       slack.SlackNotifier
}

type Options struct {
	// CacheNamespace namespace where Yale will store its cache entries
	CacheNamespace string
	// IgnoreUsageMetrics if true, Yale will NOT check if a service account is in use before disabling it
	IgnoreUsageMetrics bool
	// SlackWebhookUrl if set, Yale will send slack notifications to this webhook
	SlackWebhookUrl string
}

// NewYale /* Construct a new Yale Manager */
func NewYale(clients *client.Clients, opts ...func(*Options)) *Yale {
	return newYaleFromClients(clients.GetK8s(), clients.GetCRDs(), clients.GetIAM(), clients.GetMetrics(), clients.GetVault(), clients.GetAzure(), opts...)
}

func newYaleFromClients(k8s kubernetes.Interface, crd v1beta1.YaleCRDInterface, iam *iam.Service, metrics *monitoring.MetricClient, vault *vaultapi.Client, azure *msgraph.ApplicationsClient, opts ...func(*Options)) *Yale {
	options := Options{
		CacheNamespace:     cache.DefaultCacheNamespace,
		IgnoreUsageMetrics: false,
	}
	for _, opt := range opts {
		opt(&options)
	}
	_keyops := make(map[string]keyops.KeyOps)
	_keyops[gcpKeyops] = keyops.New(iam)
	_keyops[azureKeyops] = azurekeyops.New(azure)

	_authmetrics := authmetrics.New(metrics, iam)
	_cache := cache.New(k8s, options.CacheNamespace)
	_keysync := keysync.New(k8s, vault, _cache)
	_resourcemap := resourcemap.New(crd, _cache)
	_slack := slack.New(options.SlackWebhookUrl)

	return newYaleFromComponents(options, _cache, _resourcemap, _authmetrics, _keyops, _keysync, _slack)
}

func newYaleFromComponents(options Options, _cache cache.Cache, resourcemapper resourcemap.Mapper, _authmetrics authmetrics.AuthMetrics, _keyops map[string]keyops.KeyOps, _keysync keysync.KeySync, _slack slack.SlackNotifier) *Yale {
	return &Yale{
		options:     options,
		cache:       _cache,
		resourcemap: resourcemapper,
		authmetrics: _authmetrics,
		keyops:      _keyops,
		keysync:     _keysync,
		slack:       _slack,
	}
}

func (m *Yale) Run() error {
	resources, err := m.resourcemap.Build()
	if err != nil {
		return fmt.Errorf("error inspecting cluster for cache entries and GcpSaKey resources: %v", err)
	}

	errors := make(map[string]error)
	for identifier, bundle := range resources {
		if bundle.Entry.Identifier.Type() == cache.GcpSaKey {
			if err = m.processServiceAccountAndReportErrors(bundle.Entry, bundle.GSKs); err != nil {
				logs.Error.Printf("error processing service account %s: %v", identifier, err)
				errors[identifier] = err
			}
		} else if bundle.Entry.Identifier.Type() == cache.AzureClientSecret {
			logs.Info.Printf("processing azure client secret %s", identifier)
			if err = m.processAzureClientSecretAndReportErrors(bundle.Entry, bundle.AzClientSecrets); err != nil {
				logs.Error.Printf("error processing azure client secret %s: %v", identifier, err)
				errors[identifier] = err
			}
		}
	}

	if len(errors) > 0 {
		var sb strings.Builder
		for email, err := range errors {
			sb.WriteString(fmt.Sprintf("%s: %v\n", email, err))
		}
		return fmt.Errorf("error processing GcpSaKeys for %d service accounts: %s", len(errors), sb.String())
	}

	return nil
}

func (m *Yale) processServiceAccountAndReportErrors(entry *cache.Entry, gsks []apiv1b1.GcpSaKey) error {
	if err := m.processServiceAccount(entry, gsks); err != nil {
		if reportErr := m.reportError(entry, err); reportErr != nil {
			logs.Error.Printf("error reporting error for service account %s: %v", entry.Identify(), reportErr)
		}
		return err
	}
	return nil
}

func (m *Yale) processAzureClientSecretAndReportErrors(entry *cache.Entry, acss []apiv1b1.AzureClientSecret) error {
	if err := m.processAzureClientSecret(entry, acss); err != nil {
		if reportErr := m.reportError(entry, err); reportErr != nil {
			logs.Error.Printf("error reporting error for azure client secret %s: %v", entry.Identify(), reportErr)
		}
		return err
	}
	return nil
}

func (m *Yale) processServiceAccount(entry *cache.Entry, gsks []apiv1b1.GcpSaKey) error {
	var err error

	cutoffs := computeCutoffs(entry, gsks)

	if err = m.syncKeyIfReady(entry, gsks); err != nil {
		return err
	}
	if err = m.deleteOldKeys(entry, cutoffs); err != nil {
		return err
	}
	if err = m.disableOldKeys(entry, cutoffs); err != nil {
		return err
	}
	if err = m.rotateKey(entry, cutoffs, gsks); err != nil {
		return err
	}
	if err = retireCacheEntryIfNeeded(m.cache, entry, gsks); err != nil {
		return err
	}

	return nil
}

func (m *Yale) processAzureClientSecret(entry *cache.Entry, azureClientSecrets []apiv1b1.AzureClientSecret) error {
	var err error
	cutoffs := computeCutoffs(entry, azureClientSecrets)

	if err = m.syncClientSecretIfReady(entry, azureClientSecrets); err != nil {
		return err
	}
	if err = m.disableOldKeys(entry, cutoffs); err != nil {
		return err
	}

	if err = m.disableOldKeys(entry, cutoffs); err != nil {
		return err
	}

	if err = m.rotateClientSecret(entry, cutoffs, azureClientSecrets); err != nil {
		return err
	}

	if err = retireCacheEntryIfNeeded(m.cache, entry, azureClientSecrets); err != nil {
		return err
	}

	return nil
}

// computeCutoffs computes the cutoffs for key rotation/disabling/deletion based on the GcpSaKey resources
// for this service account
func computeCutoffs[Y apiv1b1.YaleCRD](entry *cache.Entry, yaleCRDs []Y) cutoff.Cutoffs {
	if len(yaleCRDs) == 0 {
		logs.Info.Printf("cache entry for %s has no corresponding %T resources in the cluster; will use Yale's default cutoffs to retire old keys", entry.Identify(), yaleCRDs)
		return cutoff.NewWithDefaults()
	}
	return cutoff.New(yaleCRDs)
}

// syncKeyIfReady if cache entry has a current/active key, perform a keysync
func (m *Yale) syncKeyIfReady(entry *cache.Entry, gsks []apiv1b1.GcpSaKey) error {
	return syncYaleResourceIfReady(m.keysync, entry, gsks)
}

func (m *Yale) syncClientSecretIfReady(entry *cache.Entry, azureClientSecret []apiv1b1.AzureClientSecret) error {
	return syncYaleResourceIfReady(m.keysync, entry, azureClientSecret)
}

func syncYaleResourceIfReady[Y apiv1b1.YaleCRD](keysync keysync.KeySync, entry *cache.Entry, yaleCRDs []Y) error {
	if len(entry.CurrentKey.ID) == 0 {
		// nothing to sync yet
		return nil
	}
	switch crds := any(&yaleCRDs).(type) {
	case *[]apiv1b1.GcpSaKey:
		return keysync.SyncIfNeeded(entry, *crds, nil)
	case *[]apiv1b1.AzureClientSecret:
		return keysync.SyncIfNeeded(entry, nil, *crds)
	default:
		return fmt.Errorf("unknown yaleCRD type %T", yaleCRDs)
	}
}

// rotateKey rotates the current active key for the service account, if needed.
func (m *Yale) rotateKey(entry *cache.Entry, cutoffs cutoff.Cutoffs, gsks []apiv1b1.GcpSaKey) error {
	rotated, err := m.issueNewKeyIfNeeded(entry, cutoffs, gsks)
	if err != nil {
		return err
	}
	if !rotated {
		return nil
	}
	return syncYaleResourceIfReady(m.keysync, entry, gsks)
}

func (m *Yale) rotateClientSecret(entry *cache.Entry, cutoffs cutoff.Cutoffs, azureClientSecrets []apiv1b1.AzureClientSecret) error {
	rotated, err := m.issueNewClientSecretIfNeeded(entry, cutoffs, azureClientSecrets)
	if err != nil {
		return err
	}
	if !rotated {
		return nil
	}
	return syncYaleResourceIfReady(m.keysync, entry, azureClientSecrets)
}

// issueNewKeyIfNeed given cache entry and gsk, checks if the entry's current active key needs to be rotated.
// if a rotation is needed (or the cache entry is new/empty), it issues a new sa key, adds it
// to the cache entry, then saves the updated cache entry to k8s.
// returns a boolean that will be true if a new key was issued, false otherwise
func (m *Yale) issueNewKeyIfNeeded(entry *cache.Entry, cutoffs cutoff.Cutoffs, gsks []apiv1b1.GcpSaKey) (bool, error) {
	return issueNewYaleResourceIfNeeded(m.keyops[gcpKeyops], m.cache, m.slack, entry, cutoffs, gsks)
}

func (m *Yale) issueNewClientSecretIfNeeded(entry *cache.Entry, cutoffs cutoff.Cutoffs, azureClientSecrets []apiv1b1.AzureClientSecret) (bool, error) {
	return issueNewYaleResourceIfNeeded(m.keyops[azureKeyops], m.cache, m.slack, entry, cutoffs, azureClientSecrets)
}

func issueNewYaleResourceIfNeeded[Y apiv1b1.YaleCRD](keyops keyops.KeyOps, yaleCache cache.Cache, slack slack.SlackNotifier, entry *cache.Entry, cutoffs cutoff.Cutoffs, yaleCRDs []Y) (bool, error) {
	issued := false
	identifier := entry.Identify()
	scope := entry.Scope()

	if entry.CurrentKey.ID != "" {
		logs.Info.Printf("%T %s: checking if current secret %s needs rotation (created at %s; rotation age is %d days)", entry.Type, identifier, entry.CurrentKey.ID, entry.CurrentKey.CreatedAt, cutoffs.RotateAfterDays())
		if cutoffs.ShouldRotate(entry.CurrentKey.CreatedAt) {
			logs.Info.Printf("%T %s: current secret %s needs rotation", entry.Type, identifier, entry.CurrentKey.ID)
			entry.RotatedKeys[entry.CurrentKey.ID] = currentTime()
			entry.CurrentKey = cache.CurrentKey{}
		} else {
			logs.Info.Printf("%T %s: current secret %s does not need rotation", entry.Type, identifier, entry.CurrentKey.ID)
		}
	}

	if entry.CurrentKey.ID == "" {
		if len(yaleCRDs) == 0 {
			logs.Info.Printf("%T %s: no %T resources in cluster; will not issue new key", entry.Type, identifier, yaleCRDs)
		} else {
			logs.Info.Printf("%T %s: issuing new key", entry.Type, identifier)
			newKey, secret, err := keyops.Create(scope, identifier)
			if err != nil {
				return false, fmt.Errorf("error issuing new secret for %s: %v", identifier, err)
			}
			logs.Info.Printf("%T %s: issued new secret %s", entry.Type, identifier, newKey.ID)
			entry.CurrentKey.ID = newKey.ID
			entry.CurrentKey.JSON = string(secret)
			entry.CurrentKey.CreatedAt = currentTime()
			issued = true
		}
	}

	if err := yaleCache.Save(entry); err != nil {
		return false, fmt.Errorf("error saving cache entry for %s after key rotation: %v", identifier, err)
	}

	if issued {
		if err := slack.KeyIssued(entry, entry.CurrentKey.ID); err != nil {
			return false, err
		}
	}

	return issued, nil
}

func (m *Yale) disableOldKeys(entry *cache.Entry, cutoffs cutoff.Cutoffs) error {
	for keyId, rotatedAt := range entry.RotatedKeys {
		if err := m.disableOneKey(keyId, rotatedAt, entry, cutoffs); err != nil {
			return err
		}
	}
	return nil
}

func (m *Yale) disableOneKey(keyId string, rotatedAt time.Time, entry *cache.Entry, cutoffs cutoff.Cutoffs) error {
	// has enough time passed since rotation? if not, do nothing
	_keyops := m.keyops[gcpKeyops]

	logs.Info.Printf("key %s (service account %s) was rotated at %s, disable cutoff is %d days", keyId, entry.Identify(), rotatedAt, cutoffs.DisableAfterDays())
	if !cutoffs.ShouldDisable(rotatedAt) {
		logs.Info.Printf("key %s (service account %s): too early to disable", keyId, entry.Identify())
		return nil
	}

	// check if the key is still in use
	lastAuthTime, err := m.lastAuthTime(keyId, entry)
	if err != nil {
		return err
	}
	if lastAuthTime != nil {
		if !cutoffs.SafeToDisable(*lastAuthTime) {
			return fmt.Errorf("key %s (service account %s) was rotated at %s but was last used to authenticate at %s; please find out what's still using this key and fix it", keyId, entry.Identify(), rotatedAt, *lastAuthTime)
		}
	}

	// disable the key
	logs.Info.Printf("disabling key %s (service account %s)...", keyId, entry.Identify())
	if err = _keyops.EnsureDisabled(keyops.Key{
		Scope:      entry.Scope(),
		Identifier: entry.Identify(),
		ID:         keyId,
	}); err != nil {
		return fmt.Errorf("error disabling key %s (service account %s): %v", keyId, entry.Identify(), err)
	}

	// update cache entry to reflect that the key was successfully disabled
	delete(entry.RotatedKeys, keyId)
	entry.DisabledKeys[keyId] = currentTime()
	if err = m.cache.Save(entry); err != nil {
		return fmt.Errorf("error saving cache entry after key disable: %v", err)
	}

	return m.slack.KeyDisabled(entry, keyId)
}

func (m *Yale) lastAuthTime(keyId string, entry *cache.Entry) (*time.Time, error) {
	if m.options.IgnoreUsageMetrics {
		return nil, nil
	}

	logs.Info.Printf("key %s (service account %s) has reached disable cutoff; checking if still in use", keyId, entry.Identify())
	lastAuthTime, err := m.authmetrics.LastAuthTime(entry.Scope(), entry.Identify(), keyId)
	if err != nil {
		return nil, fmt.Errorf("error determining last authentication time for key %s (service account %s): %v", keyId, entry.Identify(), err)
	}
	if lastAuthTime == nil {
		logs.Info.Printf("could not identify last authentication time for key %s (service account %s); assuming key is not in use", keyId, entry.Identify())
		return nil, nil
	}
	logs.Info.Printf("last authentication time for key %s (service account %s): %s", keyId, entry.Identify(), *lastAuthTime)
	return lastAuthTime, nil
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
	_keyops := m.keyops[gcpKeyops]
	logs.Info.Printf("key %s (service account %s) was disabled at %s, delete cutoff is %d days", keyId, entry.Identify(), disabledAt, cutoffs.DisableAfterDays())
	if !cutoffs.ShouldDelete(disabledAt) {
		logs.Info.Printf("key %s (service account %s): too early to delete", keyId, entry.Identify())
		return nil
	}

	key := keyops.Key{
		Scope:      entry.Scope(),
		Identifier: entry.Identify(),
		ID:         keyId,
	}

	// delete key from GCP
	logs.Info.Printf("key %s (service account %s) has reached delete cutoff; deleting it", key.ID, key.Identifier)
	if err := _keyops.DeleteIfDisabled(key); err != nil {
		return fmt.Errorf("error deleting key %s (service account %s): %v", keyId, entry.Identify(), err)
	}

	// delete key from cache entry
	delete(entry.DisabledKeys, keyId)
	if err := m.cache.Save(entry); err != nil {
		return fmt.Errorf("error updating cache entry for %s after key deletion: %v", entry.Identify(), err)
	}

	logs.Info.Printf("deleted key %s (service account %s)", key.ID, key.Identifier)
	return m.slack.KeyDeleted(entry, key.ID)
}

func retireCacheEntryIfNeeded[Y apiv1b1.YaleCRD](yaleCache cache.Cache, entry *cache.Entry, yaleCRDs []Y) error {
	if len(yaleCRDs) > 0 {
		return nil
	}
	if len(entry.CurrentKey.ID) > 0 {
		logs.Info.Printf("cache entry for %s has no corresponding GcpSaKey resources in the cluster; will not delete it because it still has a current key", entry.Identify())
		return nil
	}
	if len(entry.RotatedKeys) > 0 {
		logs.Info.Printf("cache entry for %s has no corresponding GcpSaKey resources in the cluster; will not delete it because it still has keys to disable", entry.Identify())
		return nil
	}
	if len(entry.DisabledKeys) > 0 {
		logs.Info.Printf("cache entry for %s has no corresponding GcpSaKey resources in the cluster; will not delete it because it still has keys to delete", entry.Identify())
		return nil
	}

	logs.Info.Printf("cache entry for %s is empty and has no corresponding GcpSaKey resources in the cluster; deleting it", entry.Identify())
	return yaleCache.Delete(entry)
}

const errorRepostDuration = 4 * time.Hour

// reportError report an error on Slack
func (m *Yale) reportError(entry *cache.Entry, err error) error {
	now := currentTime()

	entry.LastError.Message = err.Error()
	entry.LastError.Timestamp = now

	if err = m.cache.Save(entry); err != nil {
		return fmt.Errorf("error saving cache entry after recording error: %v", err)
	}

	if time.Since(entry.LastError.LastNotificationAt) < errorRepostDuration {
		return nil
	}

	if err = m.slack.Error(entry, entry.LastError.Message); err != nil {
		return fmt.Errorf("error reporting error to Slack: %v", err)
	}

	entry.LastError.LastNotificationAt = now
	if err = m.cache.Save(entry); err != nil {
		return fmt.Errorf("error saving cache entry after reporting error: %v", err)
	}

	return nil
}

// time.Now, but in a standard way that is nicer-looking in log messages and easier to test
func currentTime() time.Time {
	return time.Now().UTC().Round(0)
}
