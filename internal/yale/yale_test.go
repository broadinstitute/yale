package yale

import (
	"context"
	"fmt"
	authmetricsmocks "github.com/broadinstitute/yale/internal/yale/authmetrics/mocks"
	"github.com/broadinstitute/yale/internal/yale/cache"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/crd/clientset/v1beta1/mocks"
	"github.com/broadinstitute/yale/internal/yale/keyops"
	keyopsmocks "github.com/broadinstitute/yale/internal/yale/keyops/mocks"
	"github.com/broadinstitute/yale/internal/yale/keysync"
	vaultutils "github.com/broadinstitute/yale/internal/yale/keysync/testutils/vault"
	"github.com/broadinstitute/yale/internal/yale/resourcemap"
	"github.com/broadinstitute/yale/internal/yale/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"testing"
	"time"
)

const cacheNamespace = cache.DefaultCacheNamespace

// Define the suite, and absorb the built-in basic suite
// functionality from testify - including a T() method which
// returns the current testing context
type YaleSuite struct {
	suite.Suite
	k8s            kubernetes.Interface
	gskEndpoint    *mocks.GcpSaKeyInterface
	vaultServer    *vaultutils.FakeVaultServer
	cache          cache.Cache
	resourcemapper resourcemap.Mapper
	authmetrics    *authmetricsmocks.AuthMetrics
	keyops         *keyopsmocks.KeyOps
	keysync        keysync.KeySync
	yale           *Yale
}

func (suite *YaleSuite) SetupTest() {
	// create kubernetes clients - fake k8s client, mock gsk endpoint
	suite.k8s = testutils.NewFakeK8sClient(suite.T())
	suite.gskEndpoint = mocks.NewGcpSaKeyInterface(suite.T())
	crd := mocks.NewYaleCRDInterface(suite.T())
	crd.EXPECT().GcpSaKeys().Return(suite.gskEndpoint)

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
	suite.yale = newYaleFromComponents(suite.cache, suite.resourcemapper, suite.authmetrics, suite.keyops, suite.keysync)
}

func (suite *YaleSuite) TestYaleSucceedsWithNoCacheEntriesOrGcpSaKeys() {
	suite.seedGsks()
	require.NoError(suite.T(), suite.yale.Run())
}

var now = time.Now().UTC().Round(0)
var eightDaysAgo = now.Add(-8 * 24 * time.Hour).Round(0)
var fourDaysAgo = now.Add(-4 * 24 * time.Hour).Round(0)
var fourHoursAgo = now.Add(-4 * time.Hour).Round(0)

var sa1 = cache.ServiceAccount{
	Email:   "s1@p.com",
	Project: "p",
}

var sa2 = cache.ServiceAccount{
	Email:   "s2@p.com",
	Project: "p.com",
}

var sa3 = cache.ServiceAccount{
	Email:   "s3@p.com",
	Project: "p.com",
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

var gsk1 = apiv1b1.GCPSaKey{
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

var gsk2 = apiv1b1.GCPSaKey{
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

var gsk3 = apiv1b1.GCPSaKey{
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

func (suite *YaleSuite) TestYaleIssuesNewKeyForNewGcpSaKey() {
	suite.seedGsks(gsk1)

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

func (suite *YaleSuite) TestYaleRotatesOldKey() {
	suite.seedGsks(gsk1)

	suite.seedCacheEntries(&cache.Entry{
		ServiceAccount: sa1,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key1.id,
			JSON:      sa1key1.json(),
			CreatedAt: eightDaysAgo,
		},
	})

	suite.expectCreateKey(sa1key2)

	require.NoError(suite.T(), suite.yale.Run())

	// make sure the cache contains the new key
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), sa1key2.id, entry.CurrentKey.ID)
	assert.Equal(suite.T(), sa1key2.json(), entry.CurrentKey.JSON)
	suite.assertNow(entry.CurrentKey.CreatedAt)

	// make sure the cache entry's rotated section includes the old key
	t, exists := entry.RotatedKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	// make sure the new key was replicated to the secret in the gsk spec
	suite.assertSecretHasData("ns-1", "s1-secret", map[string]string{
		"key.pem":  sa1key2.pem,
		"key.json": sa1key2.json(),
	})
}

func (suite *YaleSuite) TestYaleDisablesOldKeyIfNotInUse() {
	suite.seedGsks(gsk1)

	suite.seedCacheEntries(&cache.Entry{
		ServiceAccount: sa1,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key2.id,
			JSON:      sa1key2.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			sa1key1.id: eightDaysAgo,
		},
	})

	suite.expectLastAuthTime(sa1key1, fourDaysAgo)
	suite.expectDisableKey(sa1key1)

	require.NoError(suite.T(), suite.yale.Run())

	// validate cache entry
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	// make sure the cache entry's rotated section does not include the old key
	_, exists := entry.RotatedKeys[sa1key1.id]
	assert.False(suite.T(), exists)

	// make sure the cache entry's disabled section includes the old key
	t, exists := entry.DisabledKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)
}

func (suite *YaleSuite) TestYaleDisablesOldKeyIfNoUsageDataAvailable() {
	suite.seedGsks(gsk1)

	suite.seedCacheEntries(&cache.Entry{
		ServiceAccount: sa1,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key2.id,
			JSON:      sa1key2.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			sa1key1.id: eightDaysAgo,
		},
	})

	suite.expectNoLastAuthTime(sa1key1)
	suite.expectDisableKey(sa1key1)

	require.NoError(suite.T(), suite.yale.Run())

	// validate cache entry
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	// make sure the cache entry's rotated section does not include the old key
	_, exists := entry.RotatedKeys[sa1key1.id]
	assert.False(suite.T(), exists)

	// make sure the cache entry's disabled section includes the old key
	t, exists := entry.DisabledKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)
}

func (suite *YaleSuite) TestYaleReturnsErrorIfOldRotatedKeyIsStillInUse() {
	suite.seedGsks(gsk1)

	suite.seedCacheEntries(&cache.Entry{
		ServiceAccount: sa1,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key2.id,
			JSON:      sa1key2.json(),
			CreatedAt: now,
		},
		RotatedKeys: map[string]time.Time{
			sa1key1.id: eightDaysAgo,
		},
	})

	suite.expectLastAuthTime(sa1key1, fourHoursAgo)

	err := suite.yale.Run()
	require.Error(suite.T(), err)
	assert.ErrorContains(suite.T(), err, "please find out what's still using this key")

	// make sure the cache still includes this key in the rotated section, not disabled
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	// make sure the cache entry's rotated section does not include the old key
	t, exists := entry.RotatedKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	assert.Equal(suite.T(), eightDaysAgo, t)

	// make sure the cache entry's disabled section includes the old key
	_, exists = entry.DisabledKeys[sa1key1.id]
	assert.False(suite.T(), exists)
}

func (suite *YaleSuite) TestYaleDoesNotRotateDisableOrDeleteKeysThatAreNotOldEnough() {
	suite.seedGsks(gsk1)

	suite.seedCacheEntries(&cache.Entry{
		ServiceAccount: sa1,
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

	require.NoError(suite.T(), suite.yale.Run())

	// validate cache entry
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	// make sure the cache entry's rotated section still includes key2
	t, exists := entry.RotatedKeys[sa1key2.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	// make sure the cache entry's disabled section still includes key1
	t, exists = entry.DisabledKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)
}

func (suite *YaleSuite) TestYaleDeletesOldKeys() {
	suite.seedGsks(gsk1)

	suite.seedCacheEntries(&cache.Entry{
		ServiceAccount: sa1,
		CurrentKey: cache.CurrentKey{
			ID:        sa1key2.id,
			JSON:      sa1key2.json(),
			CreatedAt: now,
		},
		DisabledKeys: map[string]time.Time{
			sa1key1.id: now,
		},
	})

	require.NoError(suite.T(), suite.yale.Run())

	// validate cache entry
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	// make sure the cache entry's disabled section still includes key1
	t, exists := entry.DisabledKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)
}

func (suite *YaleSuite) TestYaleCorrectlyProcessesCacheEntryWithNoMatchingGcpSaKeys() {
	suite.seedGsks()

	suite.seedCacheEntries(&cache.Entry{
		ServiceAccount: sa1,
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

	suite.expectLastAuthTime(sa1key2, eightDaysAgo)
	suite.expectDisableKey(sa1key2)
	suite.expectDeleteKey(sa1key3)

	require.NoError(suite.T(), suite.yale.Run())

	// validate cache entry
	entry, err := suite.cache.GetOrCreate(sa1)
	require.NoError(suite.T(), err)

	// make sure no replacement key was issued
	assert.Empty(suite.T(), entry.CurrentKey)

	// make sure the old current key was rotated
	assert.Len(suite.T(), entry.RotatedKeys, 1)
	t, exists := entry.RotatedKeys[sa1key1.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)

	// make sure the old rotated key was disabled
	assert.Len(suite.T(), entry.DisabledKeys, 1)
	t, exists = entry.DisabledKeys[sa1key2.id]
	assert.True(suite.T(), exists)
	suite.assertNow(t)
}

func (suite *YaleSuite) TestYaleCorrectlyRetiresCacheEntryWithNoMatchingGcpSaKeys() {
	suite.seedGsks()

	suite.seedCacheEntries(&cache.Entry{
		ServiceAccount: sa1,
		CurrentKey:     cache.CurrentKey{},
		RotatedKeys:    map[string]time.Time{},
		DisabledKeys: map[string]time.Time{
			sa1key1.id: eightDaysAgo,
		},
	})

	suite.expectDeleteKey(sa1key1)

	require.NoError(suite.T(), suite.yale.Run())

	// ensure cache entry was removed from the cluster
	entries, err := suite.cache.List()
	require.NoError(suite.T(), err)
	assert.Empty(suite.T(), entries)
}

func (suite *YaleSuite) TestYaleAggregatesAndReportsErrors() {
	suite.seedGsks(gsk1, gsk2, gsk3)

	suite.expectCreateKeyReturnsErr(sa1key1, fmt.Errorf("uh-oh"))
	suite.expectCreateKey(sa2key1)
	suite.expectCreateKeyReturnsErr(sa3key1, fmt.Errorf("oh noes"))

	err := suite.yale.Run()
	require.Error(suite.T(), err)
	assert.ErrorContains(suite.T(), err, "s1@p.com: uh-oh")
	assert.ErrorContains(suite.T(), err, "s3@p.com: oh noes")

	// make sure the cache contains the new keys for sa2
	entry, err := suite.cache.GetOrCreate(sa2)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), sa2key1.id, entry.CurrentKey.ID)
	assert.Equal(suite.T(), sa2key1.json(), entry.CurrentKey.JSON)
	suite.assertNow(entry.CurrentKey.CreatedAt)

	// make sure the new keys were replicated to the secret in the gsk spec
	suite.assertSecretHasData("ns-2", "s2-secret", map[string]string{
		"key.pem":  sa2key1.pem,
		"key.json": sa2key1.json(),
	})
}

func (suite *YaleSuite) seedGsks(gsks ...apiv1b1.GCPSaKey) {
	suite.gskEndpoint.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(&apiv1b1.GCPSaKeyList{
		Items: gsks,
	}, nil)
}

func (suite *YaleSuite) seedCacheEntries(entries ...*cache.Entry) {
	// the cache doesn't have a function for bulk adding a bunch of new entries into it,
	// so this is a little awkward.
	for _, e := range entries {
		_, err := suite.cache.GetOrCreate(e.ServiceAccount)
		require.NoError(suite.T(), err)
		err = suite.cache.Save(e)
		require.NoError(suite.T(), err)
	}
}

func (suite *YaleSuite) expectCreateKeyReturnsErr(k key, err error) {
	suite.keyops.EXPECT().Create(k.sa.Project, k.sa.Email).Return(k.keyopsFormat(), []byte(k.json()), err)
}

func (suite *YaleSuite) expectCreateKey(k key) {
	suite.keyops.EXPECT().Create(k.sa.Project, k.sa.Email).Return(k.keyopsFormat(), []byte(k.json()), nil)
}

func (suite *YaleSuite) expectDisableKey(k key) {
	suite.keyops.EXPECT().EnsureDisabled(k.keyopsFormat()).Return(nil)
}

func (suite *YaleSuite) expectDeleteKey(k key) {
	suite.keyops.EXPECT().DeleteIfDisabled(k.keyopsFormat()).Return(nil)
}

func (suite *YaleSuite) expectLastAuthTime(k key, t time.Time) {
	suite.authmetrics.EXPECT().LastAuthTime(k.sa.Project, k.sa.Email, k.id).Return(&t, nil)
}

func (suite *YaleSuite) expectNoLastAuthTime(k key) {
	suite.authmetrics.EXPECT().LastAuthTime(k.sa.Project, k.sa.Email, k.id).Return(nil, nil)
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
	sa  cache.ServiceAccount
	pem string
}

func (k key) keyopsFormat() keyops.Key {
	return keyops.Key{
		ID:                  k.id,
		ServiceAccountEmail: k.sa.Email,
		Project:             k.sa.Project,
	}
}

func (k key) json() string {
	return `{"email":"` + k.sa.Email + `","private_key":"` + k.pem + `"}`
}
