package cache

import (
	"context"
	"encoding/json"
	"github.com/broadinstitute/yale/internal/yale/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"testing"
	"time"
)

const namespace = "my-cache-namespace"
const project = "my-project"

var sa1 = ServiceAccount{
	Email:   "my-sa1@p.com",
	Project: project,
}
var sa2 = ServiceAccount{
	Email:   "my-sa2@p.com",
	Project: project,
}
var sa3 = ServiceAccount{
	Email:   "my-sa3@p.com",
	Project: project,
}

func Test_Cache(t *testing.T) {
	k8s := testutils.NewFakeK8sClient(t)
	cache := New(k8s, namespace)

	// make sure secret does not exist, to start
	secret := readCacheSecret(t, k8s, sa1.cacheSecretName())
	require.Nil(t, secret)

	// make sure we get an empty list
	entries, err := cache.List()
	require.NoError(t, err)
	assert.Len(t, entries, 0)

	// create new empty cache entry
	expected := emptyCacheEntry(sa1)
	entry, err := cache.GetOrCreate(sa1)
	require.NoError(t, err)
	assert.Equal(t, &expected, entry)

	// make sure the underlying secret was created with the attributes we expect
	secret = readCacheSecret(t, k8s, sa1.cacheSecretName())
	require.NotNil(t, secret)
	expectedContent, err := json.Marshal(expected)
	require.NoError(t, err)
	assert.Equal(t, sa1.cacheSecretName(), secret.Name)
	assert.Equal(t, namespace, secret.Namespace)
	assert.Equal(t, labelValue, secret.Labels[labelKey])
	assert.Equal(t, string(expectedContent), string(secret.Data[secretKey]))

	// reading the entry again should yield a copy of the entry with identical data
	entryCopy, err := cache.GetOrCreate(sa1)
	require.NoError(t, err)
	assert.Equal(t, entry, entryCopy)

	now := time.Now().Round(0).UTC()

	// updating and saving entry should persist the changes
	entry.CurrentKey.ID = "key-1"
	entry.CurrentKey.CreatedAt = now
	entry.CurrentKey.JSON = `{"foo":"bar"}`

	entry.RotatedKeys["key-2"] = now
	entry.RotatedKeys["key-3"] = now
	entry.DisabledKeys["key-4"] = now
	entry.SyncStatus["my-ns/my-gsk"] = "my-sha256-sum:key-1"

	require.NoError(t, cache.Save(entry))

	// make sure saving the cache entry did not overwrite any of the fields we set on the entry object
	assert.Equal(t, "key-1", entry.CurrentKey.ID)
	assert.Equal(t, now, entry.CurrentKey.CreatedAt)
	assert.Equal(t, `{"foo":"bar"}`, entry.CurrentKey.JSON)
	assert.Equal(t, now, entry.RotatedKeys["key-2"])
	assert.Equal(t, now, entry.RotatedKeys["key-3"])
	assert.Equal(t, now, entry.DisabledKeys["key-4"])
	assert.Equal(t, "my-sha256-sum:key-1", entry.SyncStatus["my-ns/my-gsk"])

	// reading the entry again should yield a copy of the entry with identical data
	entryCopy, err = cache.GetOrCreate(sa1)
	require.NoError(t, err)
	assert.Equal(t, entry, entryCopy)

	// listing all cache entries should yield just the entry we created
	entries, err = cache.List()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, entry, entries[0])

	// add 2 more cache entries
	entry2, err := cache.GetOrCreate(sa2)
	require.NoError(t, err)
	assert.Equal(t, emptyCacheEntry(sa2), *entry2)

	entry3, err := cache.GetOrCreate(sa3)
	require.NoError(t, err)
	assert.Equal(t, emptyCacheEntry(sa3), *entry3)

	// make sure updates to entry3 persist
	entry3.CurrentKey.ID = "e3-key3"
	require.NoError(t, cache.Save(entry3))

	entry3Copy, err := cache.GetOrCreate(sa3)
	require.NoError(t, err)
	assert.Equal(t, "e3-key3", entry3Copy.CurrentKey.ID)

	// make sure all entries appear in the list
	entries, err = cache.List()
	assert.Len(t, entries, 3)
	assert.Equal(t, entry, entries[0])
	assert.Equal(t, entry2, entries[1])
	assert.Equal(t, entry3, entries[2])

	// delete entry2
	require.NoError(t, cache.Delete(entry2))
	entries, err = cache.List()

	// make sure entry and entry3 appear in the list
	assert.Len(t, entries, 2)
	assert.Equal(t, entry, entries[0])
	assert.Equal(t, entry3, entries[1])

	// delete first entry
	require.NoError(t, cache.Delete(entry))

	// make sure just entry3 appears in the list
	entries, err = cache.List()
	assert.Len(t, entries, 1)
	assert.Equal(t, entry3, entries[0])

	// delete entry3
	require.NoError(t, cache.Delete(entry3))

	// make sure list is empty again
	entries, err = cache.List()
	assert.Len(t, entries, 0)

	// get or create new entry for the same sa as a deleted entry should create a new empty entry
	entry, err = cache.GetOrCreate(sa1)
	require.NoError(t, err)
	assert.Equal(t, emptyCacheEntry(sa1), *entry)

	// list should return error if a cache entry exists with invalid data
	// create a cache entry with invalid data
	_, err = k8s.CoreV1().Secrets(namespace).Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "invalid-fake-cache-entry",
			Labels: map[string]string{
				labelKey: labelValue,
			},
		},
		Data: map[string][]byte{
			secretKey: []byte(`{}`), // no service account information!
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// list should return error
	_, err = cache.List()
	assert.ErrorContains(t, err, "missing service account email")
}

func Test_cacheSecretName(t *testing.T) {
	assert.Equal(t, "yale-cache-my-sa1-p.com", sa1.cacheSecretName())
}

func readCacheSecret(t *testing.T, k8s kubernetes.Interface, name string) *corev1.Secret {
	secret, err := k8s.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)
	return secret
}

// represents the expected initial state of a new/empty cache entry
func emptyCacheEntry(sa ServiceAccount) Entry {
	return Entry{
		ServiceAccount: ServiceAccount{
			Email:   sa.Email,
			Project: sa.Project,
		},
		CurrentKey: struct {
			JSON      string
			ID        string
			CreatedAt time.Time
		}{},
		// we expect _empty_ maps, not nil maps
		RotatedKeys:  map[string]time.Time{},
		DisabledKeys: map[string]time.Time{},
		SyncStatus:   map[string]string{},
	}
}
