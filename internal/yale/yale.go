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

type RotateWindow struct {
	Enabled   bool
	StartTime time.Time
	EndTime   time.Time
}

type Options struct {
	// CacheNamespace namespace where Yale will store its cache entries
	CacheNamespace string
	// IgnoreUsageMetrics if true, Yale will NOT check if a service account is in use before disabling it
	IgnoreUsageMetrics bool
	// SlackWebhookUrl if set, Yale will send Slack notifications to this webhook
	SlackWebhookUrl string
	// RotateWindow if enabled, restrict key rotation operations to a specific time of day
	RotateWindow RotateWindow
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

// Run is the main entrypoint for Yale, and will perform a full sync of all yale-managed resources in the cluster
func (m *Yale) Run() error {
	resources, err := m.resourcemap.Build()
	if err != nil {
		return fmt.Errorf("error inspecting cluster for cache entries and GcpSaKey resources: %v", err)
	}

	errors := make(map[string]error)
	for identifier, bundle := range resources {
		logs.Info.Printf("processing %s %s", bundle.Entry.Type, identifier)
		if bundle.Entry.Identifier.Type() == cache.GcpSaKey {
			if err = processYaleResourceAndReportErrors(m, bundle.Entry, bundle.GSKs); err != nil {
				logs.Error.Printf("error processing %s %s: %v", bundle.Entry.Type, identifier, err)
				errors[identifier] = err
			}
		} else if bundle.Entry.Identifier.Type() == cache.AzureClientSecret {

			if err = processYaleResourceAndReportErrors(m, bundle.Entry, bundle.AzClientSecrets); err != nil {
				logs.Error.Printf("error processing %s %s: %v", bundle.Entry.Type, identifier, err)
				errors[identifier] = err
			}
		}
	}

	if len(errors) > 0 {
		var sb strings.Builder
		for email, err := range errors {
			sb.WriteString(fmt.Sprintf("%s: %v\n", email, err))
		}
		return fmt.Errorf("error processing yale managed resource for %d identifier: %s", len(errors), sb.String())
	}

	return nil
}

// processYaleResourceAndReportErrors is a helper function that will process a Yale-managed resource, and report any errors that occur
func processYaleResourceAndReportErrors[Y apiv1b1.YaleCRD](yale *Yale, entry *cache.Entry, yaleCRDs []Y) error {
	if err := processYaleResource(yale, entry, yaleCRDs); err != nil {
		if reportErr := yale.reportError(entry, err); reportErr != nil {
			logs.Error.Printf("error reporting error for %s: %v", entry.Identify(), reportErr)
		}
		return err
	}
	return nil
}

// processYaleResource is a helper function that will process a Yale-managed resource
func processYaleResource[Y apiv1b1.YaleCRD](yale *Yale, entry *cache.Entry, yaleCRDs []Y) error {
	var err error
	var keyOpsType string
	if entry.Type == cache.GcpSaKey {
		keyOpsType = gcpKeyops
	} else if entry.Type == cache.AzureClientSecret {
		keyOpsType = azureKeyops
	} else {
		return fmt.Errorf("unknown entry type %T", entry.Type)
	}

	cutoffs := computeCutoffs(entry, yaleCRDs)

	if err = syncYaleResourceIfReady(yale.keysync, entry, yaleCRDs); err != nil {
		return err
	}

	if err = issueNewYaleResourceIfNoCurrent(yale.keyops[keyOpsType], yale.cache, yale.keysync, yale.slack, entry, yaleCRDs); err != nil {
		return err
	}

	window := yale.options.RotateWindow
	if window.Enabled {
		if currentTime().Before(window.StartTime) || currentTime().After(window.EndTime) {
			logs.Info.Printf("won't attempt key rotations for %s %s because we are outside the rotation window (%s - %s)", entry.Type, entry.Identifier, window.StartTime, window.EndTime)
			return nil
		}
	}

	if err = yale.deleteOldKeys(entry, cutoffs); err != nil {
		return err
	}
	if err = yale.disableOldKeys(entry, cutoffs); err != nil {
		return err
	}
	if err = rotateYaleResourceIfNeeded(yale.keyops[keyOpsType], yale.cache, yale.keysync, yale.slack, entry, cutoffs, yaleCRDs); err != nil {
		return err
	}
	if err = retireCacheEntryIfNeeded(yale.cache, entry, yaleCRDs); err != nil {
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

// syncYaleResourceIfReady will sync the active key for a cache entry if it exists to the keysync destination
func syncYaleResourceIfReady[Y apiv1b1.YaleCRD](_keysync keysync.KeySync, entry *cache.Entry, yaleCRDs []Y) error {
	if len(entry.CurrentKey.ID) == 0 {
		// nothing to sync yet
		return nil
	}
	switch crds := any(&yaleCRDs).(type) {
	case *[]apiv1b1.GcpSaKey:
		return _keysync.SyncIfNeeded(entry, keysync.GcpSaKeysToSyncable(*crds))
	case *[]apiv1b1.AzureClientSecret:
		return _keysync.SyncIfNeeded(entry, keysync.AzureClientSecretsToSyncable(*crds))
	default:
		return fmt.Errorf("unknown yaleCRD type %T", yaleCRDs)
	}
}

// rotateYaleResourceIfNeeded if a cache entry needs rotation, rotate it and kick off a keysync
func rotateYaleResourceIfNeeded[Y apiv1b1.YaleCRD](
	keyops keyops.KeyOps,
	yaleCache cache.Cache,
	keysync keysync.KeySync,
	slack slack.SlackNotifier,
	entry *cache.Entry,
	cutoffs cutoff.Cutoffs,
	yaleCRDs []Y,
) error {
	identifier := entry.Identify()

	// check if we actually need to issue a new key
	if entry.CurrentKey.ID == "" {
		if len(yaleCRDs) == 0 {
			logs.Info.Printf("%s %s: no %T resources in cluster; will not issue new key", entry.Type, identifier, yaleCRDs)
			return nil
		}
		logs.Info.Printf("%s %s: no current secret; will issue new key", entry.Type, identifier)
	} else {
		// there IS a current key already, so check if it needs rotation
		logs.Info.Printf("%s %s: checking if current secret %s needs rotation (created at %s; rotation age is %d days)", entry.Type, identifier, entry.CurrentKey.ID, entry.CurrentKey.CreatedAt, cutoffs.RotateAfterDays())
		if !cutoffs.ShouldRotate(entry.CurrentKey.CreatedAt) {
			logs.Info.Printf("%s %s: current secret %s does not need rotation; will not issue new key", entry.Type, identifier, entry.CurrentKey.ID)
			return nil
		}
		// key is expired, but no CRDs in the cluster, so mark it rotated *without* issuing a new key
		if len(yaleCRDs) == 0 {
			// mark the current key for rotation
			logs.Info.Printf("%s %s: no %T resources in cluster; moving expired current key to rotated", entry.Type, identifier, yaleCRDs)
			entry.RotatedKeys = map[string]time.Time{entry.CurrentKey.ID: currentTime()}
			entry.CurrentKey = cache.CurrentKey{}
			if err := yaleCache.Save(entry); err != nil {
				return fmt.Errorf("error saving cache entry for %s: %v", identifier, err)
			}
			return nil
		}
		logs.Info.Printf("%s %s: current secret %s needs rotation; will issue new key", entry.Type, identifier, entry.CurrentKey.ID)
	}

	// issue new key
	logs.Info.Printf("%s %s: issuing new key", entry.Type, identifier)
	if err := issueNewYaleResource(keyops, yaleCache, slack, entry); err != nil {
		return fmt.Errorf("error issuing new secret for %s: %v", identifier, err)
	}

	return syncYaleResourceIfReady(keysync, entry, yaleCRDs)
}

// issueNewYaleResourceIfNoCurrent if cache entry has no current value, issue a new secret and kick off a keysync
func issueNewYaleResourceIfNoCurrent[Y apiv1b1.YaleCRD](
	keyops keyops.KeyOps,
	yaleCache cache.Cache,
	keysync keysync.KeySync,
	slack slack.SlackNotifier,
	entry *cache.Entry,
	yaleCRDs []Y,
) error {
	identifier := entry.Identify()

	// check if we actually need to issue a new key
	if entry.CurrentKey.ID != "" {
		return nil
	}
	if len(yaleCRDs) == 0 {
		logs.Info.Printf("%s %s: no current secret, but no %T resources in cluster; will not issue new key", entry.Type, identifier, yaleCRDs)
		return nil
	}

	logs.Info.Printf("%s %s: no current secret; will issue new key", entry.Type, identifier)
	if err := issueNewYaleResource(keyops, yaleCache, slack, entry); err != nil {
		return fmt.Errorf("%s %s: error issuing new secret: %v", entry.Type, identifier, err)
	}
	return syncYaleResourceIfReady(keysync, entry, yaleCRDs)
}

// issueNewYaleResource issues a new secret, adds it to the cache entry,
// saves the updated cache entry to k8s, and sends a Slack notification
func issueNewYaleResource(
	keyops keyops.KeyOps,
	yaleCache cache.Cache,
	slack slack.SlackNotifier,
	entry *cache.Entry,
) error {
	identifier := entry.Identify()
	scope := entry.Scope()

	// issue new key
	logs.Info.Printf("%s %s: issuing new secret...", entry.Type, identifier)
	newKey, secret, err := keyops.Create(scope, identifier)
	if err != nil {
		return fmt.Errorf("error issuing new secret for %s: %v", identifier, err)
	}
	logs.Info.Printf("%s %s: issued new secret %s", entry.Type, identifier, newKey.ID)

	// update the cache entry with our new secret
	if entry.CurrentKey.ID != "" {
		// mark the current key for rotation if there is one
		entry.RotatedKeys[entry.CurrentKey.ID] = currentTime()
	}
	entry.CurrentKey = cache.CurrentKey{
		ID:        newKey.ID,
		JSON:      string(secret),
		CreatedAt: currentTime(),
	}
	if err = yaleCache.Save(entry); err != nil {
		return fmt.Errorf("error saving cache entry for %s after key rotation: %v", identifier, err)
	}

	// send Slack notification that we issued a new key
	if err = slack.KeyIssued(entry, entry.CurrentKey.ID); err != nil {
		return err
	}

	return nil
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

	logs.Info.Printf("key %s (%s %s) was rotated at %s, disable cutoff is %d days", keyId, entry.Type, entry.Identify(), rotatedAt, cutoffs.DisableAfterDays())
	if !cutoffs.ShouldDisable(rotatedAt) {
		logs.Info.Printf("key %s (%s %s): too early to disable", keyId, entry.Type, entry.Identify())
		return nil
	}

	// check if the key is still in use
	lastAuthTime, err := m.lastAuthTime(keyId, entry)
	if err != nil {
		return err
	}
	if lastAuthTime != nil {
		if !cutoffs.SafeToDisable(*lastAuthTime) {
			return fmt.Errorf("key %s (%s %s) was rotated at %s but was last used to authenticate at %s; please find out what's still using this key and fix it", keyId, entry.Type, entry.Identify(), rotatedAt, *lastAuthTime)
		}
	}

	// disable the key
	logs.Info.Printf("disabling key %s (%s %s)...", keyId, entry.Type, entry.Identify())
	if err = _keyops.EnsureDisabled(keyops.Key{
		Scope:      entry.Scope(),
		Identifier: entry.Identify(),
		ID:         keyId,
	}); err != nil {
		return fmt.Errorf("error disabling key %s (%s %s): %v", keyId, entry.Type, entry.Identify(), err)
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

	logs.Info.Printf("key %s (%s %s) has reached disable cutoff; checking if still in use", keyId, entry.Type, entry.Identify())
	lastAuthTime, err := m.authmetrics.LastAuthTime(entry.Scope(), entry.Identify(), keyId)
	if err != nil {
		return nil, fmt.Errorf("error determining last authentication time for key %s (%s %s): %v", keyId, entry.Type, entry.Identify(), err)
	}
	if lastAuthTime == nil {
		logs.Info.Printf("could not identify last authentication time for key %s (%s %s); assuming key is not in use", keyId, entry.Type, entry.Identify())
		return nil, nil
	}
	logs.Info.Printf("last authentication time for key %s (%s %s): %s", keyId, entry.Type, entry.Identify(), *lastAuthTime)
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
	logs.Info.Printf("key %s (%s %s) was disabled at %s, delete cutoff is %d days", keyId, entry.Type, entry.Identify(), disabledAt, cutoffs.DisableAfterDays())
	if !cutoffs.ShouldDelete(disabledAt) {
		logs.Info.Printf("key %s (%s %s): too early to delete", keyId, entry.Type, entry.Identify())
		return nil
	}

	key := keyops.Key{
		Scope:      entry.Scope(),
		Identifier: entry.Identify(),
		ID:         keyId,
	}

	// delete key from GCP
	logs.Info.Printf("key %s (%s %s) has reached delete cutoff; deleting it", key.ID, entry.Type, key.Identifier)
	if err := _keyops.DeleteIfDisabled(key); err != nil {
		return fmt.Errorf("error deleting key %s (%s %s): %v", keyId, entry.Type, entry.Identify(), err)
	}

	// delete key from cache entry
	delete(entry.DisabledKeys, keyId)
	if err := m.cache.Save(entry); err != nil {
		return fmt.Errorf("error updating cache entry for %s after key deletion: %v", entry.Identify(), err)
	}

	logs.Info.Printf("deleted key %s (%s %s)", key.ID, entry.Type, key.Identifier)
	return m.slack.KeyDeleted(entry, key.ID)
}

func retireCacheEntryIfNeeded[Y apiv1b1.YaleCRD](yaleCache cache.Cache, entry *cache.Entry, yaleCRDs []Y) error {
	if len(yaleCRDs) > 0 {
		return nil
	}
	if len(entry.CurrentKey.ID) > 0 {
		logs.Info.Printf("cache entry for %s has no corresponding %s resources in the cluster; will not delete it because it still has a current key", entry.Identify(), entry.Type)
		return nil
	}
	if len(entry.RotatedKeys) > 0 {
		logs.Info.Printf("cache entry for %s has no corresponding %s resources in the cluster; will not delete it because it still has keys to disable", entry.Identify(), entry.Type)
		return nil
	}
	if len(entry.DisabledKeys) > 0 {
		logs.Info.Printf("cache entry for %s has no corresponding %s resources in the cluster; will not delete it because it still has keys to delete", entry.Identify(), entry.Type)
		return nil
	}

	logs.Info.Printf("cache entry for %s is empty and has no corresponding %s resources in the cluster; deleting it", entry.Identify(), entry.Type)
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
