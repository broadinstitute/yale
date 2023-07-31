package yale

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	authmetricsmocks "github.com/broadinstitute/yale/internal/yale/authmetrics/mocks"
	"github.com/broadinstitute/yale/internal/yale/cache"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	crdmocks "github.com/broadinstitute/yale/internal/yale/crd/clientset/v1beta1/mocks"
	"github.com/broadinstitute/yale/internal/yale/keyops"
	keyopsmocks "github.com/broadinstitute/yale/internal/yale/keyops/mocks"
	"github.com/broadinstitute/yale/internal/yale/keysync"
	vaultutils "github.com/broadinstitute/yale/internal/yale/keysync/testutils/vault"
	"github.com/broadinstitute/yale/internal/yale/resourcemap"
	"github.com/broadinstitute/yale/internal/yale/slack"
	slackmocks "github.com/broadinstitute/yale/internal/yale/slack/mocks"
	"github.com/broadinstitute/yale/internal/yale/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const cacheNamespace = cache.DefaultCacheNamespace

// Define the suite, and absorb the built-in basic suite
// functionality from testify - including a T() method which
// returns the current testing context
type YaleSuite struct {
	suite.Suite
	k8s                    kubernetes.Interface
	gskEndpoint            *crdmocks.GcpSaKeyInterface
	azClientSecretEndpoint *crdmocks.AzureClientSecretInterface
	vaultServer            *vaultutils.FakeVaultServer
	cache                  cache.Cache
	resourcemapper         resourcemap.Mapper
	authmetrics            *authmetricsmocks.AuthMetrics
	keyops                 *keyopsmocks.KeyOps
	keysync                keysync.KeySync
	slack                  slack.SlackNotifier
	yale                   *Yale
}

func (suite *YaleSuite) SetupTest() {
	// create kubernetes clients - fake k8s client, mock gsk endpoint
	suite.k8s = testutils.NewFakeK8sClient(suite.T())
	suite.gskEndpoint = crdmocks.NewGcpSaKeyInterface(suite.T())
	suite.azClientSecretEndpoint = crdmocks.NewAzureClientSecretInterface(suite.T())
	crd := crdmocks.NewYaleCRDInterface(suite.T())
	crd.EXPECT().GcpSaKeys().Return(suite.gskEndpoint)
	crd.EXPECT().AzureClientSecrets().Return(suite.azClientSecretEndpoint)

	suite.vaultServer = vaultutils.NewFakeVaultServer(suite.T())

	// use real suite.cache and suite.resourcemapper instead of mocks.
	// Lots of things write to the cache and instead of mocking all
	// the intermediate cache entry writes during a Yale run,
	// it's much easier just to verify cache state at the end
	suite.cache = cache.New(suite.k8s, cacheNamespace)
	suite.resourcemapper = resourcemap.New(crd, suite.cache)

	// use mocks for these, since mocking gcp api calls is a pain
	suite.authmetrics = authmetricsmocks.NewAuthMetrics(suite.T())
	suite.keyops = keyopsmocks.NewKeyOps(suite.T())

	// use real keysync so we can verify the state of Vault server/K8s secrets
	// after the yale run finishes, without mocking every individual call
	suite.keysync = keysync.New(suite.k8s, suite.vaultServer.NewClient(), suite.cache)

	// use noop slack notifier
	suite.slack = slack.New("")

	// store mock keyops in a map[string]Keyops, so that application logic can switch between
	// different keyops backends
	_keyops := make(map[string]keyops.KeyOps)
	// use mock implementations for both keyops instances
	_keyops[gcpKeyops] = suite.keyops
	_keyops[azureKeyops] = suite.keyops

	suite.yale = newYaleFromComponents(
		Options{
			CacheNamespace:     cache.DefaultCacheNamespace,
			IgnoreUsageMetrics: false,
			RotateWindow: RotateWindow{
				Enabled: true,
				// Make sure the current time is inside the rotation window
				StartTime: currentTime().Add(-1 * time.Hour),
				EndTime:   currentTime().Add(time.Hour),
			},
		},
		suite.cache,
		suite.resourcemapper,
		suite.authmetrics,
		_keyops,
		suite.keysync,
		suite.slack,
	)
}

func (suite *YaleSuite) TestYaleSucceedsWithNoCacheEntriesOrGcpSaKeys() {
	suite.seedGsks()
	suite.seedAzureClientSecrets()
	require.NoError(suite.T(), suite.yale.Run())
}

var now = currentTime()
var eightDaysAgo = now.Add(-8 * 24 * time.Hour).Round(0)
var fourDaysAgo = now.Add(-4 * 24 * time.Hour).Round(0)
var fourHoursAgo = now.Add(-4 * time.Hour).Round(0)

var sa1 = cache.GcpSaKeyEntryIdentifier{
	Email:   "s1@p.com",
	Project: "p",
}

var sa2 = cache.GcpSaKeyEntryIdentifier{
	Email:   "s2@p.com",
	Project: "p.com",
}

var sa3 = cache.GcpSaKeyEntryIdentifier{
	Email:   "s3@p.com",
	Project: "p.com",
}

var clientSecret1 = cache.AzureClientSecretEntryIdentifier{
	ApplicationID: "test-app-id-1",
	TenantID:      "test-tenant-id",
}

var clientSecret2 = cache.AzureClientSecretEntryIdentifier{
	ApplicationID: "test-app-id-2",
	TenantID:      "test-tenant-id",
}

var clientSecret3 = cache.AzureClientSecretEntryIdentifier{
	ApplicationID: "test-app-id-3",
	TenantID:      "test-tenant-id",
}

var sa1key1 = key{
	id:  "s1-key1",
	sa:  sa1,
	pem: "foo",
}

var sa1key2 = key{
	id:  "s1-key2",
	sa:  sa1,
	pem: "bar",
}

var sa1key3 = key{
	id:  "s1-key3",
	sa:  sa1,
	pem: "baz",
}

var sa2key1 = key{
	id:  "s2-key1",
	sa:  sa2,
	pem: "cat",
}

var sa3key1 = key{
	id:  "s3-key1",
	sa:  sa3,
	pem: "dog",
}

var clientSecret1Key1 = key{
	id:  "cs1-key1",
	sa:  clientSecret1,
	pem: "az-secret",
}

var clientSecret1Key2 = key{
	id:  "cs1-key2",
	sa:  clientSecret1,
	pem: "az-seceret2",
}

var clientSecret1Key3 = key{
	id:  "cs1-key3",
	sa:  clientSecret1,
	pem: "az-secret3",
}

var clientSecret2Key1 = key{
	id:  "cs2-key1",
	sa:  clientSecret2,
	pem: "az-secret",
}

var clientSecret3Key1 = key{
	id:  "cs3-key1",
	sa:  clientSecret3,
	pem: "az-secret",
}

var gsk1 = apiv1b1.GcpSaKey{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "s1-gsk",
		Namespace: "ns-1",
	},
	Spec: apiv1b1.GCPSaKeySpec{
		GoogleServiceAccount: apiv1b1.GoogleServiceAccount{
			Name:    sa1.Email,
			Project: sa1.Project,
		},
		KeyRotation: apiv1b1.KeyRotation{
			RotateAfter:  7,
			DisableAfter: 7,
			DeleteAfter:  3,
		},
		Secret: apiv1b1.Secret{
			Name:        "s1-secret",
			PemKeyName:  "key.pem",
			JsonKeyName: "key.json",
		},
	},
}

var gsk2 = apiv1b1.GcpSaKey{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "s2-gsk",
		Namespace: "ns-2",
	},
	Spec: apiv1b1.GCPSaKeySpec{
		GoogleServiceAccount: apiv1b1.GoogleServiceAccount{
			Name:    sa2.Email,
			Project: sa2.Project,
		},
		KeyRotation: apiv1b1.KeyRotation{
			RotateAfter:  7,
			DisableAfter: 7,
			DeleteAfter:  3,
		},
		Secret: apiv1b1.Secret{
			Name:        "s2-secret",
			PemKeyName:  "key.pem",
			JsonKeyName: "key.json",
		},
	},
}

var gsk3 = apiv1b1.GcpSaKey{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "s3-gsk",
		Namespace: "ns-3",
	},
	Spec: apiv1b1.GCPSaKeySpec{
		GoogleServiceAccount: apiv1b1.GoogleServiceAccount{
			Name:    sa3.Email,
			Project: sa3.Project,
		},
		KeyRotation: apiv1b1.KeyRotation{
			RotateAfter:  7,
			DisableAfter: 7,
			DeleteAfter:  3,
		},
		Secret: apiv1b1.Secret{
			Name:        "s3-secret",
			PemKeyName:  "key.pem",
			JsonKeyName: "key.json",
		},
	},
}

var acs1 = apiv1b1.AzureClientSecret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "clientsecret1-acs",
		Namespace: "ns-1",
	},
	Spec: apiv1b1.AzureClientSecretSpec{
		AzureServicePrincipal: apiv1b1.AzureServicePrincipal{
			ApplicationID: clientSecret1.ApplicationID,
			TenantID:      clientSecret1.TenantID,
		},
		KeyRotation: apiv1b1.KeyRotation{
			RotateAfter:  7,
			DisableAfter: 7,
			DeleteAfter:  3,
		},
		Secret: apiv1b1.Secret{
			Name:                "clientsecret1-secret",
			ClientSecretKeyName: "clientsecret-key",
		},
	},
}

var acs2 = apiv1b1.AzureClientSecret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "clientsecret2-acs",
		Namespace: "ns-2",
	},
	Spec: apiv1b1.AzureClientSecretSpec{
		AzureServicePrincipal: apiv1b1.AzureServicePrincipal{
			ApplicationID: clientSecret2.ApplicationID,
			TenantID:      clientSecret2.TenantID,
		},
		KeyRotation: apiv1b1.KeyRotation{
			RotateAfter:  7,
			DisableAfter: 7,
			DeleteAfter:  3,
		},
		Secret: apiv1b1.Secret{
			Name:                "clientsecret2-secret",
			ClientSecretKeyName: "clientsecret-key",
		},
	},
}

var acs3 = apiv1b1.AzureClientSecret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "clientsecret3-acs",
		Namespace: "ns-3",
	},
	Spec: apiv1b1.AzureClientSecretSpec{
		AzureServicePrincipal: apiv1b1.AzureServicePrincipal{
			ApplicationID: clientSecret3.ApplicationID,
			TenantID:      clientSecret3.TenantID,
		},
		KeyRotation: apiv1b1.KeyRotation{
			RotateAfter:  7,
			DisableAfter: 7,
			DeleteAfter:  3,
		},
		Secret: apiv1b1.Secret{
			Name:                "clientsecret3-secret",
			ClientSecretKeyName: "clientsecret-key",
		},
	},
}

func (suite *YaleSuite) TestYaleIssuesNewKeyForNewGcpSaKey() {
	suite.seedGsks(gsk1)
	suite.seedAzureClientSecrets()

	suite.expectCreateKey(sa1key1)

	require.NoError(suite.T(), suite.yale.Run())

	// make sure the cache contains the new key
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), sa1key1.id, entry.CurrentKey.ID)
	assert.Equal(suite.T(), sa1key1.json(), entry.CurrentKey.JSON)
	suite.assertNow(entry.CurrentKey.CreatedAt)

	// make sure the new key was replicated to the secret in the gsk spec
	suite.assertSecretHasData("ns-1", "s1-secret", map[string]string{
		"key.pem":  sa1key1.pem,
		"key.json": sa1key1.json(),
	})
}

func (suite *YaleSuite) TestYaleIssuesNewSecretButDoesNotRotateIfOutsideRotationWindow() {
	_keyops := make(map[string]keyops.KeyOps)
	// use mock implementations for both keyops instances
	_keyops[gcpKeyops] = suite.keyops
	_keyops[azureKeyops] = suite.keyops

	suite.yale = newYaleFromComponents(
		Options{
			CacheNamespace:     cache.DefaultCacheNamespace,
			IgnoreUsageMetrics: false,
			RotateWindow: RotateWindow{
				Enabled:   true,
				StartTime: currentTime().Add(time.Hour),
				EndTime:   currentTime().Add(2 * time.Hour),
			},
		},
		suite.cache,
		suite.resourcemapper,
		suite.authmetrics,
		_keyops,
		suite.keysync,
		suite.slack,
	)

	suite.seedGsks(gsk1, gsk2)
	suite.seedAzureClientSecrets()

	suite.seedCacheEntries(&cache.Entry{
		Identifier: sa2,
		Type:       cache.GcpSaKey,
		CurrentKey: cache.CurrentKey{
			ID:        sa2key1.id,
			JSON:      sa2key1.json(),
			CreatedAt: eightDaysAgo,
		},
	})

	suite.expectCreateKey(sa1key1)

	// Note: we do NOT expect a create key operation for sa2 because it is outside the rotation window

	require.NoError(suite.T(), suite.yale.Run())

	// make sure the cache contains the new key for sa1
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), sa1key1.id, entry.CurrentKey.ID)
	assert.Equal(suite.T(), sa1key1.json(), entry.CurrentKey.JSON)
	suite.assertNow(entry.CurrentKey.CreatedAt)

	// make sure the new key was replicated to the secret in the gsk spec
	suite.assertSecretHasData("ns-1", "s1-secret", map[string]string{
		"key.pem":  sa1key1.pem,
		"key.json": sa1key1.json(),
	})
}

func (suite *YaleSuite) TestYaleIssuesNewClientSecretForNewAzureClientSecret() {
	suite.seedGsks()
	suite.seedAzureClientSecrets(acs1)
	suite.expectCreateKey(clientSecret1Key1)

	require.NoError(suite.T(), suite.yale.Run())

	// ensure cache contains new client secret
	entry, err := suite.cache.GetOrCreate(clientSecret1)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), clientSecret1Key1.id, entry.CurrentKey.ID)
	assert.Equal(suite.T(), clientSecret1Key1.json(), entry.CurrentKey.JSON)
	suite.assertNow(entry.CurrentKey.CreatedAt)

	suite.assertSecretHasData("ns-1", "clientsecret1-secret", map[string]string{
		"clientsecret-key": clientSecret1Key1.json(),
	})
}

func (suite *YaleSuite) TestYaleIssuesNewSecretsForMultipleResourceTypes() {
	suite.seedGsks(gsk1)
	suite.seedAzureClientSecrets(acs1)

	suite.expectCreateKey(sa1key1)
	suite.expectCreateKey(clientSecret1Key1)

	require.NoError(suite.T(), suite.yale.Run())

	// ensure cache contains new client secret
	entry, err := suite.cache.GetOrCreate(clientSecret1)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), clientSecret1Key1.id, entry.CurrentKey.ID)
	assert.Equal(suite.T(), clientSecret1Key1.json(), entry.CurrentKey.JSON)
	suite.assertNow(entry.CurrentKey.CreatedAt)

	// make sure the cache contains the new key
	entry, err = suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), sa1key1.id, entry.CurrentKey.ID)
	assert.Equal(suite.T(), sa1key1.json(), entry.CurrentKey.JSON)
	suite.assertNow(entry.CurrentKey.CreatedAt)

	// make sure the new key was replicated to the secret in the gsk spec
	suite.assertSecretHasData("ns-1", "s1-secret", map[string]string{
		"key.pem":  sa1key1.pem,
		"key.json": sa1key1.json(),
	})

	suite.assertSecretHasData("ns-1", "clientsecret1-secret", map[string]string{
		"clientsecret-key": clientSecret1Key1.json(),
	})
}

func (suite *YaleSuite) TestYaleRotatesOldKey() {
	suite.seedGsks(gsk1)
	suite.seedAzureClientSecrets(acs1)

	suite.seedCacheEntries(&cache.Entry{
		Identifier: sa1,
		Type:       cache.GcpSaKey,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key1.id,
			JSON:      sa1key1.json(),
			CreatedAt: eightDaysAgo,
		},
	})

	suite.seedCacheEntries(&cache.Entry{
		Identifier: clientSecret1,
		Type:       cache.AzureClientSecret,
		CurrentKey: cache.CurrentKey{
			ID:        clientSecret1Key1.id,
			JSON:      clientSecret1Key1.json(),
			CreatedAt: eightDaysAgo,
		},
	})

	suite.expectCreateKey(sa1key2)
	suite.expectCreateKey(clientSecret1Key2)

	require.NoError(suite.T(), suite.yale.Run())

	// make sure the cache contains the new key
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	entryAcs, err := suite.cache.GetOrCreate(clientSecret1)
	require.NoError(suite.T(), err)

	assert.Equal(suite.T(), sa1key2.id, entry.CurrentKey.ID)
	assert.Equal(suite.T(), sa1key2.json(), entry.CurrentKey.JSON)
	suite.assertNow(entry.CurrentKey.CreatedAt)

	assert.Equal(suite.T(), clientSecret1Key2.id, entryAcs.CurrentKey.ID)
	assert.Equal(suite.T(), clientSecret1Key2.json(), entryAcs.CurrentKey.JSON)
	suite.assertNow(entryAcs.CurrentKey.CreatedAt)

	// make sure the cache entry's rotated section includes the old key
	t, exists := entry.RotatedKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	t, exists = entryAcs.RotatedKeys[clientSecret1Key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	// make sure the new key was replicated to the secret in the gsk spec
	suite.assertSecretHasData("ns-1", "s1-secret", map[string]string{
		"key.pem":  sa1key2.pem,
		"key.json": sa1key2.json(),
	})

	suite.assertSecretHasData("ns-1", "clientsecret1-secret", map[string]string{
		"clientsecret-key": clientSecret1Key2.json(),
	})
}

func (suite *YaleSuite) TestYaleDisablesOldKeyIfNotInUse() {
	suite.seedGsks(gsk1)
	suite.seedAzureClientSecrets(acs1)

	suite.seedCacheEntries(&cache.Entry{
		Identifier: sa1,
		Type:       cache.GcpSaKey,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key2.id,
			JSON:      sa1key2.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			sa1key1.id: eightDaysAgo,
		},
	})

	suite.seedCacheEntries(&cache.Entry{
		Identifier: clientSecret1,
		Type:       cache.AzureClientSecret,
		CurrentKey: cache.CurrentKey{
			ID:        clientSecret1Key2.id,
			JSON:      clientSecret1Key2.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			clientSecret1Key1.id: eightDaysAgo,
		},
	})

	suite.expectLastAuthTime(sa1key1, fourDaysAgo)
	suite.expectLastAuthTime(clientSecret1Key1, fourDaysAgo)
	suite.expectDisableKey(sa1key1)
	suite.expectDisableKey(clientSecret1Key1)

	require.NoError(suite.T(), suite.yale.Run())

	// validate cache entry
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	entryAcs, err := suite.cache.GetOrCreate(clientSecret1)
	require.NoError(suite.T(), err)

	// make sure the cache entry's rotated section does not include the old key
	_, exists := entry.RotatedKeys[sa1key1.id]
	assert.False(suite.T(), exists)

	_, exists = entryAcs.RotatedKeys[clientSecret1Key1.id]
	assert.False(suite.T(), exists)

	// make sure the cache entry's disabled section includes the old key
	t, exists := entry.DisabledKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	t, exists = entryAcs.DisabledKeys[clientSecret1Key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)
}

func (suite *YaleSuite) TestYaleDisablesOldKeyIfNoUsageDataAvailable() {
	suite.seedGsks(gsk1)
	suite.seedAzureClientSecrets(acs1)

	suite.seedCacheEntries(&cache.Entry{
		Identifier: sa1,
		Type:       cache.GcpSaKey,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key2.id,
			JSON:      sa1key2.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			sa1key1.id: eightDaysAgo,
		},
	})

	suite.seedCacheEntries(&cache.Entry{
		Identifier: clientSecret1,
		Type:       cache.AzureClientSecret,
		CurrentKey: cache.CurrentKey{
			ID:        clientSecret1Key2.id,
			JSON:      clientSecret1Key2.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			clientSecret1Key1.id: eightDaysAgo,
		},
	})

	suite.expectNoLastAuthTime(sa1key1)
	suite.expectDisableKey(sa1key1)

	suite.expectNoLastAuthTime(clientSecret1Key1)
	suite.expectDisableKey(clientSecret1Key1)

	require.NoError(suite.T(), suite.yale.Run())

	// validate cache entry
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	entryAcs, err := suite.cache.GetOrCreate(clientSecret1)
	require.NoError(suite.T(), err)

	// make sure the cache entry's rotated section does not include the old key
	_, exists := entry.RotatedKeys[sa1key1.id]
	assert.False(suite.T(), exists)

	_, exists = entryAcs.RotatedKeys[clientSecret1Key1.id]
	assert.False(suite.T(), exists)

	// make sure the cache entry's disabled section includes the old key
	t, exists := entry.DisabledKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	t, exists = entryAcs.DisabledKeys[clientSecret1Key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)
}

func (suite *YaleSuite) TestYaleReturnsErrorIfOldRotatedKeyIsStillInUse() {
	suite.seedGsks(gsk1)
	suite.seedAzureClientSecrets(acs1)

	suite.seedCacheEntries(&cache.Entry{
		Identifier: sa1,
		Type:       cache.GcpSaKey,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key2.id,
			JSON:      sa1key2.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			sa1key1.id: eightDaysAgo,
		},
	})

	suite.seedCacheEntries(&cache.Entry{
		Identifier: clientSecret1,
		Type:       cache.AzureClientSecret,
		CurrentKey: cache.CurrentKey{
			ID:        clientSecret1Key2.id,
			JSON:      clientSecret1Key2.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			clientSecret1Key1.id: eightDaysAgo,
		},
	})

	suite.expectLastAuthTime(sa1key1, fourHoursAgo)
	suite.expectLastAuthTime(clientSecret1Key1, fourHoursAgo)

	err := suite.yale.Run()
	require.Error(suite.T(), err)
	assert.ErrorContains(suite.T(), err, "please find out what's still using this key")

	// make sure the cache still includes this key in the rotated section, not disabled
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	entryAcs, err := suite.cache.GetOrCreate(clientSecret1)
	require.NoError(suite.T(), err)

	// make sure the cache entry's rotated section does not include the old key
	t, exists := entry.RotatedKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	assert.Equal(suite.T(), eightDaysAgo, t)

	t, exists = entryAcs.RotatedKeys[clientSecret1Key1.id]
	assert.True(suite.T(), exists)
	assert.Equal(suite.T(), eightDaysAgo, t)

	// make sure the cache entry's disabled section includes the old key
	_, exists = entry.DisabledKeys[sa1key1.id]
	assert.False(suite.T(), exists)

	_, exists = entryAcs.DisabledKeys[clientSecret1Key1.id]
	assert.False(suite.T(), exists)
}

func (suite *YaleSuite) TestYaleDoesNotCheckIfRotatedKeyIsStillInUseIfIgnoreUsageMetricsIsTrue() {
	_keyops := make(map[string]keyops.KeyOps)
	_keyops[gcpKeyops] = suite.keyops
	_keyops[azureKeyops] = suite.keyops
	// overwrite default yale instance with one where IgnoreUsageMetrics is true
	suite.yale = newYaleFromComponents(
		Options{
			CacheNamespace:     cache.DefaultCacheNamespace,
			IgnoreUsageMetrics: true,
		},
		suite.cache,
		suite.resourcemapper,
		suite.authmetrics,
		_keyops,
		suite.keysync,
		suite.slack,
	)

	suite.seedGsks(gsk1)
	suite.seedAzureClientSecrets(acs1)

	suite.seedCacheEntries(&cache.Entry{
		Identifier: sa1,
		Type:       cache.GcpSaKey,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key2.id,
			JSON:      sa1key2.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			sa1key1.id: eightDaysAgo,
		},
	})

	suite.seedCacheEntries(&cache.Entry{
		Identifier: clientSecret1,
		Type:       cache.AzureClientSecret,
		CurrentKey: cache.CurrentKey{
			ID:        clientSecret1Key2.id,
			JSON:      clientSecret1Key2.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			clientSecret1Key1.id: eightDaysAgo,
		},
	})

	// note: we intentionally don't use suite.expectLastAuthTime to set up a mock - we expect it to NOT be called it
	suite.expectDisableKey(sa1key1)
	suite.expectDisableKey(clientSecret1Key1)

	err := suite.yale.Run()
	require.NoError(suite.T(), err)

	// make sure the cache has this key in the disabled section, not rotated
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	entryAcs, err := suite.cache.GetOrCreate(clientSecret1)
	require.NoError(suite.T(), err)

	// make sure the cache entry's rotated section does not include the old key
	_, exists := entry.RotatedKeys[sa1key1.id]
	assert.False(suite.T(), exists)

	_, exists = entryAcs.RotatedKeys[clientSecret1Key1.id]
	assert.False(suite.T(), exists)

	// make sure the cache entry's disabled section includes the old key
	t, exists := entry.DisabledKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	t, exists = entryAcs.DisabledKeys[clientSecret1Key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)
}

func (suite *YaleSuite) TestYaleDoesNotRotateDisableOrDeleteKeysThatAreNotOldEnough() {
	suite.seedGsks(gsk1)
	suite.seedAzureClientSecrets(acs1)

	suite.seedCacheEntries(&cache.Entry{
		Identifier: sa1,
		Type:       cache.GcpSaKey,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key3.id,
			JSON:      sa1key3.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			sa1key2.id: now,
		},
		DisabledKeys: map[string]time.Time{
			sa1key1.id: now,
		},
	})

	suite.seedCacheEntries(&cache.Entry{
		Identifier: clientSecret1,
		Type:       cache.AzureClientSecret,
		CurrentKey: cache.CurrentKey{
			ID:        clientSecret1Key3.id,
			JSON:      clientSecret1Key3.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			clientSecret1Key2.id: now,
		},
		DisabledKeys: map[string]time.Time{
			clientSecret1Key1.id: now,
		},
	})

	require.NoError(suite.T(), suite.yale.Run())

	// validate cache entry
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	entryAcs, err := suite.cache.GetOrCreate(clientSecret1)
	require.NoError(suite.T(), err)

	// make sure the cache entry's rotated section still includes key2
	t, exists := entry.RotatedKeys[sa1key2.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	t, exists = entryAcs.RotatedKeys[clientSecret1Key2.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	// make sure the cache entry's disabled section still includes key1
	t, exists = entry.DisabledKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	t, exists = entryAcs.DisabledKeys[clientSecret1Key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)
}

func (suite *YaleSuite) TestYaleDeletesOldKeys() {
	suite.seedGsks(gsk1)
	suite.seedAzureClientSecrets(acs1)

	suite.seedCacheEntries(&cache.Entry{
		Identifier: sa1,
		Type:       cache.GcpSaKey,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key2.id,
			JSON:      sa1key2.json(),
			CreatedAt: now,
		},
		DisabledKeys: map[string]time.Time{
			sa1key1.id: eightDaysAgo,
		},
	})

	suite.seedCacheEntries(&cache.Entry{
		Identifier: clientSecret1,
		Type:       cache.AzureClientSecret,
		CurrentKey: cache.CurrentKey{
			ID:        clientSecret1Key2.id,
			JSON:      clientSecret1Key2.json(),
			CreatedAt: now,
		},
		DisabledKeys: map[string]time.Time{
			clientSecret1Key1.id: eightDaysAgo,
		},
	})

	suite.expectDeleteKey(sa1key1)
	suite.expectDeleteKey(clientSecret1Key1)

	require.NoError(suite.T(), suite.yale.Run())

	// validate cache entry
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	entryAcs, err := suite.cache.GetOrCreate(clientSecret1)
	require.NoError(suite.T(), err)

	// make sure the cache entry's disabled section is empty
	assert.Empty(suite.T(), entry.DisabledKeys)
	assert.Empty(suite.T(), entryAcs.DisabledKeys)
}

func (suite *YaleSuite) TestYaleCorrectlyProcessesCacheEntryWithNoMatchingYaleCRDs() {
	suite.seedGsks()
	suite.seedAzureClientSecrets()

	suite.seedCacheEntries(&cache.Entry{
		Identifier: sa1,
		Type:       cache.GcpSaKey,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key1.id,
			JSON:      sa1key1.json(),
			CreatedAt: eightDaysAgo,
		},
		RotatedKeys: map[string]time.Time{
			sa1key2.id: eightDaysAgo,
		},
		DisabledKeys: map[string]time.Time{
			sa1key3.id: eightDaysAgo,
		},
	})

	suite.seedCacheEntries(&cache.Entry{
		Identifier: clientSecret1,
		Type:       cache.AzureClientSecret,
		CurrentKey: cache.CurrentKey{
			ID:        clientSecret1Key1.id,
			JSON:      clientSecret1Key1.json(),
			CreatedAt: eightDaysAgo,
		},
		RotatedKeys: map[string]time.Time{
			clientSecret1Key2.id: eightDaysAgo,
		},
		DisabledKeys: map[string]time.Time{
			clientSecret1Key3.id: eightDaysAgo,
		},
	})

	suite.expectLastAuthTime(sa1key2, eightDaysAgo)
	suite.expectDisableKey(sa1key2)
	suite.expectDeleteKey(sa1key3)

	suite.expectLastAuthTime(clientSecret1Key2, eightDaysAgo)
	suite.expectDisableKey(clientSecret1Key2)
	suite.expectDeleteKey(clientSecret1Key3)

	require.NoError(suite.T(), suite.yale.Run())

	// validate cache entry
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	entryAcs, err := suite.cache.GetOrCreate(clientSecret1)
	require.NoError(suite.T(), err)

	// make sure no replacement key was issued
	assert.Empty(suite.T(), entry.CurrentKey)
	assert.Empty(suite.T(), entryAcs.CurrentKey)

	// make sure the old current key was rotated
	assert.Len(suite.T(), entry.RotatedKeys, 1)
	t, exists := entry.RotatedKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	assert.Len(suite.T(), entryAcs.RotatedKeys, 1)
	t, exists = entryAcs.RotatedKeys[clientSecret1Key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	// make sure the old rotated key was disabled
	assert.Len(suite.T(), entry.DisabledKeys, 1)
	t, exists = entry.DisabledKeys[sa1key2.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	assert.Len(suite.T(), entryAcs.DisabledKeys, 1)
	t, exists = entryAcs.DisabledKeys[clientSecret1Key2.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)
}

func (suite *YaleSuite) TestYaleCorrectlyRetiresCacheEntryWithNoMatchingGcpSaKeys() {
	suite.seedGsks()
	suite.seedAzureClientSecrets()

	suite.seedCacheEntries(&cache.Entry{
		Identifier:  sa1,
		Type:        cache.GcpSaKey,
		CurrentKey:  cache.CurrentKey{},
		RotatedKeys: map[string]time.Time{},
		DisabledKeys: map[string]time.Time{
			sa1key1.id: eightDaysAgo,
		},
	})

	suite.seedCacheEntries(&cache.Entry{
		Identifier:  clientSecret1,
		Type:        cache.AzureClientSecret,
		CurrentKey:  cache.CurrentKey{},
		RotatedKeys: map[string]time.Time{},
		DisabledKeys: map[string]time.Time{
			clientSecret1Key1.id: eightDaysAgo,
		},
	})

	suite.expectDeleteKey(sa1key1)
	suite.expectDeleteKey(clientSecret1Key1)

	require.NoError(suite.T(), suite.yale.Run())

	// ensure cache entry was removed from the cluster
	entries, err := suite.cache.List()
	require.NoError(suite.T(), err)
	assert.Empty(suite.T(), entries)
}

func (suite *YaleSuite) TestYaleAggregatesAndReportsErrors() {
	_keyops := make(map[string]keyops.KeyOps)
	_keyops[gcpKeyops] = suite.keyops
	_keyops[azureKeyops] = suite.keyops
	// overwrite default yale instance with one where slack client is a mock
	_slack := slackmocks.NewSlackNotifier(suite.T())
	suite.yale = newYaleFromComponents(
		Options{
			CacheNamespace:     cache.DefaultCacheNamespace,
			IgnoreUsageMetrics: false,
		},
		suite.cache,
		suite.resourcemapper,
		suite.authmetrics,
		_keyops,
		suite.keysync,
		_slack,
	)
	suite.seedGsks(gsk1, gsk2, gsk3)
	suite.seedAzureClientSecrets(acs1, acs2, acs3)

	suite.expectCreateKeyReturnsErr(sa1key1, fmt.Errorf("uh-oh"))
	suite.expectCreateKey(sa2key1)
	suite.expectCreateKeyReturnsErr(sa3key1, fmt.Errorf("oh noes"))

	suite.expectCreateKeyReturnsErr(clientSecret1Key1, fmt.Errorf("uh-oh"))
	suite.expectCreateKey(clientSecret2Key1)
	suite.expectCreateKeyReturnsErr(clientSecret3Key1, fmt.Errorf("oh noes"))

	lastNotification := now.Add(-20 * time.Minute)
	suite.seedCacheEntries(&cache.Entry{
		Identifier:   sa3,
		Type:         cache.GcpSaKey,
		CurrentKey:   cache.CurrentKey{},
		RotatedKeys:  map[string]time.Time{},
		DisabledKeys: map[string]time.Time{},
		LastError: cache.LastError{
			Message:            "error issuing new secret for s3@p.com: oh noes",
			Timestamp:          lastNotification,
			LastNotificationAt: lastNotification,
		},
	})

	suite.seedCacheEntries(&cache.Entry{
		Identifier:   clientSecret3,
		Type:         cache.AzureClientSecret,
		CurrentKey:   cache.CurrentKey{},
		RotatedKeys:  map[string]time.Time{},
		DisabledKeys: map[string]time.Time{},
		LastError: cache.LastError{
			Message:            "error issuing new secret for test-app-id-3: oh noes",
			Timestamp:          lastNotification,
			LastNotificationAt: lastNotification,
		},
	})

	// expect that a key issue notification is sent for sa2key1
	_slack.EXPECT().KeyIssued(mock.Anything, sa2key1.id).Return(nil)
	// set expectation that yale notifies for the s1 error (but not s3)
	_slack.EXPECT().Error(mock.Anything, mock.MatchedBy(func(s string) bool {
		return strings.HasSuffix(s, "error issuing new secret for s1@p.com: uh-oh")
	})).Return(nil)

	_slack.EXPECT().KeyIssued(mock.Anything, clientSecret2Key1.id).Return(nil)
	_slack.EXPECT().Error(mock.Anything, mock.MatchedBy(func(s string) bool {
		return strings.HasSuffix(s, "error issuing new secret for test-app-id-1: uh-oh")
	})).Return(nil)

	err := suite.yale.Run()
	require.Error(suite.T(), err)
	assert.ErrorContains(suite.T(), err, "s1@p.com: uh-oh")
	assert.ErrorContains(suite.T(), err, "s3@p.com: oh noes")
	assert.ErrorContains(suite.T(), err, "test-app-id-1: uh-oh")
	assert.ErrorContains(suite.T(), err, "test-app-id-3: oh noes")

	// make sure the cache contains the new keys for sa2
	entry, err := suite.cache.GetOrCreate(sa2)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), sa2key1.id, entry.CurrentKey.ID)
	assert.Equal(suite.T(), sa2key1.json(), entry.CurrentKey.JSON)
	suite.assertNow(entry.CurrentKey.CreatedAt)
	assert.Empty(suite.T(), entry.LastError)

	entryAcs, err := suite.cache.GetOrCreate(clientSecret2)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), clientSecret2Key1.id, entryAcs.CurrentKey.ID)
	assert.Equal(suite.T(), clientSecret2Key1.json(), entryAcs.CurrentKey.JSON)
	suite.assertNow(entryAcs.CurrentKey.CreatedAt)
	assert.Empty(suite.T(), entryAcs.LastError)

	// make sure the new key were replicated to the secret in the gsk spec
	suite.assertSecretHasData("ns-2", "s2-secret", map[string]string{
		"key.pem":  sa2key1.pem,
		"key.json": sa2key1.json(),
	})

	suite.assertSecretHasData("ns-2", "clientsecret2-secret", map[string]string{
		"clientsecret-key": clientSecret2Key1.json(),
	})

	// make sure the cache entries for s1 and s3 have error information
	entry, err = suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "GcpSaKey s1@p.com: error issuing new secret: error issuing new secret for s1@p.com: uh-oh", entry.LastError.Message)
	suite.assertNow(entry.LastError.Timestamp)
	suite.assertNow(entry.LastError.LastNotificationAt)

	entryAcs, err = suite.cache.GetOrCreate(clientSecret1)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "AzureClientSecret test-app-id-1: error issuing new secret: error issuing new secret for test-app-id-1: uh-oh", entryAcs.LastError.Message)
	suite.assertNow(entryAcs.LastError.Timestamp)
	suite.assertNow(entryAcs.LastError.LastNotificationAt)

	// s3 should NOT have sent an error, because it was already sent recently
	entry, err = suite.cache.GetOrCreate(sa3)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "GcpSaKey s3@p.com: error issuing new secret: error issuing new secret for s3@p.com: oh noes", entry.LastError.Message)
	suite.assertNow(entry.LastError.Timestamp)
	assert.Equal(suite.T(), lastNotification, entry.LastError.LastNotificationAt)

	entryAcs, err = suite.cache.GetOrCreate(clientSecret3)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "AzureClientSecret test-app-id-3: error issuing new secret: error issuing new secret for test-app-id-3: oh noes", entryAcs.LastError.Message)
	suite.assertNow(entryAcs.LastError.Timestamp)
	assert.Equal(suite.T(), lastNotification, entryAcs.LastError.LastNotificationAt)

}

func (suite *YaleSuite) seedGsks(gsks ...apiv1b1.GcpSaKey) {
	suite.gskEndpoint.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(&apiv1b1.GCPSaKeyList{
		Items: gsks,
	}, nil)
}

func (suite *YaleSuite) seedAzureClientSecrets(azClientSecrets ...apiv1b1.AzureClientSecret) {
	suite.azClientSecretEndpoint.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(&apiv1b1.AzureClientSecretList{
		Items: azClientSecrets,
	}, nil)
}

func (suite *YaleSuite) seedCacheEntries(entries ...*cache.Entry) {
	// the cache doesn't have a function for bulk adding a bunch of new entries into it,
	// so this is a little awkward.
	for _, e := range entries {
		_, err := suite.cache.GetOrCreate(e.Identifier)
		require.NoError(suite.T(), err)
		err = suite.cache.Save(e)
		require.NoError(suite.T(), err)
	}
}

func (suite *YaleSuite) expectCreateKeyReturnsErr(k key, err error) {
	suite.keyops.EXPECT().Create(k.sa.Scope(), k.sa.Identify()).Return(k.keyopsFormat(), []byte(k.json()), err)
}

func (suite *YaleSuite) expectCreateKey(k key) {
	suite.keyops.EXPECT().Create(k.sa.Scope(), k.sa.Identify()).Return(k.keyopsFormat(), []byte(k.json()), nil)
}

func (suite *YaleSuite) expectDisableKey(k key) {
	suite.keyops.EXPECT().EnsureDisabled(k.keyopsFormat()).Return(nil)
}

func (suite *YaleSuite) expectDeleteKey(k key) {
	suite.keyops.EXPECT().DeleteIfDisabled(k.keyopsFormat()).Return(nil)
}

func (suite *YaleSuite) expectLastAuthTime(k key, t time.Time) {
	suite.authmetrics.EXPECT().LastAuthTime(k.sa.Scope(), k.sa.Identify(), k.id).Return(&t, nil)
}

func (suite *YaleSuite) expectNoLastAuthTime(k key) {
	suite.authmetrics.EXPECT().LastAuthTime(k.sa.Scope(), k.sa.Identify(), k.id).Return(nil, nil)
}

func (suite *YaleSuite) assertSecretHasData(namespace string, name string, data map[string]string) {
	secret, err := suite.k8s.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(suite.T(), err, "secret %s/%s", namespace, name)

	for k, v := range data {
		assert.Equal(suite.T(), []byte(v), secret.Data[k], "secret %s/%s: field %s should have value %s", namespace, name, k, v)
	}
}

// assert a time.Time is within 5 seconds of now
func (suite *YaleSuite) assertNow(t time.Time) {
	assert.WithinDuration(suite.T(), now, t, 5*time.Second)
}

func TestYaleTestSuite(t *testing.T) {
	suite.Run(t, new(YaleSuite))
}

type key struct {
	id  string
	sa  cache.Identifier
	pem string
}

func (k key) keyopsFormat() keyops.Key {
	return keyops.Key{
		ID:         k.id,
		Identifier: k.sa.Identify(),
		Scope:      k.sa.Scope(),
	}
}

func (k key) json() string {
	if k.sa.Type() == cache.GcpSaKey {
		return `{"email":"` + k.sa.Identify() + `","private_key":"` + k.pem + `"}`
	} else {
		return k.pem
	}
}
