package keysync

import (
	"context"
	"github.com/broadinstitute/yale/internal/yale/cache"
	cachemocks "github.com/broadinstitute/yale/internal/yale/cache/mocks"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	vaultutils "github.com/broadinstitute/yale/internal/yale/keysync/testutils/vault"
	"github.com/broadinstitute/yale/internal/yale/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"testing"
)

type fakeKey struct {
	id     string
	email  string
	json   string
	pem    string
	base64 string
}

var key1 = fakeKey{
	id:     "my-key-id",
	email:  "my-sa@my-project.com",
	json:   `{"email":"my-sa@my-project.com","private_key":"foobar"}`,
	pem:    "foobar",
	base64: "eyJlbWFpbCI6Im15LXNhQG15LXByb2plY3QuY29tIiwicHJpdmF0ZV9rZXkiOiJmb29iYXIifQ==",
}

type KeySyncSuite struct {
	suite.Suite
	k8s         kubernetes.Interface
	vaultServer *vaultutils.FakeVaultServer
	cache       *cachemocks.Cache
	keysync     KeySync
}

func TestKeySyncSuite(t *testing.T) {
	suite.Run(t, new(KeySyncSuite))
}

func (suite *KeySyncSuite) SetupTest() {
	suite.k8s = testutils.NewFakeK8sClient(suite.T())
	suite.vaultServer = vaultutils.NewFakeVaultServer(suite.T())
	suite.cache = cachemocks.NewCache(suite.T())
	suite.keysync = New(suite.k8s, suite.vaultServer.NewClient(), suite.cache)
}

func (suite *KeySyncSuite) Test_KeySync_CreatesK8sSecret() {
	entry := &cache.Entry{}
	entry.CurrentKey.JSON = key1.json
	entry.CurrentKey.ID = key1.id
	entry.SyncStatus = map[string]string{} // no prior syncs recorded in the map

	gsk := apiv1b1.GCPSaKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gsk",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
		Spec: apiv1b1.GCPSaKeySpec{
			Secret: apiv1b1.Secret{
				Name:        "my-secret",
				PemKeyName:  "my-key.pem",
				JsonKeyName: "my-key.json",
			},
			VaultReplications: []apiv1b1.VaultReplication{},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)

	suite.assertK8sSecreDoesNotExist("my-namespace", "my-secret")

	// run a key sync
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, gsk))

	secret, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	// make sure secret has the correct ownership reference
	assert.Equal(suite.T(), "my-gsk", secret.OwnerReferences[0].Name)

	// make sure secret inherited labels from gsk
	assert.Equal(suite.T(), map[string]string{
		"label1": "value1",
		"label2": "value2",
	}, secret.Labels)

	// make sure secret has reloader annotations
	assert.Equal(suite.T(), "true", secret.Annotations["reloader.stakater.com/match"])

	// make sure secret has expected data
	assert.Equal(suite.T(), key1.json, string(secret.Data["my-key.json"]))
	assert.Equal(suite.T(), key1.pem, string(secret.Data["my-key.pem"]))

	// make sure the cache entry was updated with correct key-sync record
	assert.Len(suite.T(), entry.SyncStatus, 1)
	assert.Equal(suite.T(), "515a2a04abd78d13b0df1e4bc0163e1a787439fd968f364794083fa995fed009:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
}

func (suite *KeySyncSuite) Test_KeySync_UpdatesK8sSecretIfAlreadyExists() {
	entry := &cache.Entry{}
	entry.CurrentKey.JSON = key1.json
	entry.CurrentKey.ID = key1.id
	entry.SyncStatus = map[string]string{} // no prior syncs recorded in the map

	gsk := apiv1b1.GCPSaKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gsk",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
		Spec: apiv1b1.GCPSaKeySpec{
			Secret: apiv1b1.Secret{
				Name:        "my-secret",
				PemKeyName:  "my-key.pem",
				JsonKeyName: "my-key.json",
			},
			VaultReplications: []apiv1b1.VaultReplication{},
		},
	}

	suite.createSecret(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1":      "this should be overwritten",
				"extra-label": "this should be ignored",
			},
		},
		Data: map[string][]byte{
			"my-key.pem": []byte("this should be overwritten"),
			"extra-data": []byte("this should be ignored"),
		},
	})

	suite.cache.EXPECT().Save(entry).Return(nil)

	// run a key sync to create the secret once
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, gsk))

	secret, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	// make sure secret inherited labels from gsk
	assert.Equal(suite.T(), map[string]string{
		"label1":      "value1",
		"label2":      "value2",
		"extra-label": "this should be ignored",
	}, secret.Labels)

	// make sure secret has reloader annotations
	assert.Equal(suite.T(), "true", secret.Annotations["reloader.stakater.com/match"])

	// make sure secret has expected data
	assert.Equal(suite.T(), key1.json, string(secret.Data["my-key.json"]))
	assert.Equal(suite.T(), key1.pem, string(secret.Data["my-key.pem"]))
	assert.Equal(suite.T(), "this should be ignored", string(secret.Data["extra-data"]))

	// make sure the cache entry was updated with correct key-sync record
	assert.Len(suite.T(), entry.SyncStatus, 1)
	assert.Equal(suite.T(), "515a2a04abd78d13b0df1e4bc0163e1a787439fd968f364794083fa995fed009:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
}

func (suite *KeySyncSuite) Test_KeySync_PerformsAllConfiguredVaultReplications() {
	entry := &cache.Entry{}
	entry.CurrentKey.JSON = key1.json
	entry.CurrentKey.ID = key1.id
	entry.SyncStatus = map[string]string{} // no prior syncs recorded in the map

	gsk := apiv1b1.GCPSaKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gsk",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
		Spec: apiv1b1.GCPSaKeySpec{
			Secret: apiv1b1.Secret{
				Name:        "my-secret",
				PemKeyName:  "my-key.pem",
				JsonKeyName: "my-key.json",
			},
			VaultReplications: []apiv1b1.VaultReplication{
				{
					Path:   "secret/foo/test/map",
					Format: apiv1b1.Map,
				},
				{
					Path:   "secret/foo/test/json",
					Format: apiv1b1.JSON,
					Key:    "key.json",
				},
				{
					Path:   "secret/foo/test/base64",
					Format: apiv1b1.Base64,
					Key:    "key.b64",
				},
				{
					Path:   "secret/foo/test/pem",
					Format: apiv1b1.PEM,
					Key:    "key.pem",
				},
			},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)

	// run a key sync to create the K8s secret and perform the vault replications
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, gsk))

	// verify K8s secret was created
	_, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	// verify all the Vault replications were performed
	suite.assertVaultServerHasSecret("secret/foo/test/map", map[string]interface{}{
		"email":       key1.email,
		"private_key": key1.pem,
	})
	suite.assertVaultServerHasSecret("secret/foo/test/json", map[string]interface{}{
		"key.json": key1.json,
	})
	suite.assertVaultServerHasSecret("secret/foo/test/base64", map[string]interface{}{
		"key.b64": key1.base64,
	})
	suite.assertVaultServerHasSecret("secret/foo/test/pem", map[string]interface{}{
		"key.pem": key1.pem,
	})

	// make sure the cache entry was updated with correct key-sync record
	assert.Len(suite.T(), entry.SyncStatus, 1)
	assert.Equal(suite.T(), "89fee4211aee14f33a50bfd71bd47b2459560693a4548ca079a4c9d3d6b48337:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
}

func (suite *KeySyncSuite) Test_KeySync_PerformsASyncIfSyncStatusIsUpToDateButSecretIsMissing() {
	entry := &cache.Entry{}
	entry.CurrentKey.JSON = key1.json
	entry.CurrentKey.ID = key1.id

	// pretend cache entry has already been synced for this gsk
	entry.SyncStatus = map[string]string{
		"my-namespace/my-gsk": "515a2a04abd78d13b0df1e4bc0163e1a787439fd968f364794083fa995fed009:" + key1.id,
	}

	gsk := apiv1b1.GCPSaKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gsk",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
		Spec: apiv1b1.GCPSaKeySpec{
			Secret: apiv1b1.Secret{
				Name:        "my-secret",
				PemKeyName:  "my-key.pem",
				JsonKeyName: "my-key.json",
			},
			VaultReplications: []apiv1b1.VaultReplication{},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)

	// run a key sync to create the secret once
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, gsk))

	secret, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	// make sure secret has expected data
	assert.Equal(suite.T(), key1.json, string(secret.Data["my-key.json"]))
	assert.Equal(suite.T(), key1.pem, string(secret.Data["my-key.pem"]))
}

func (suite *KeySyncSuite) Test_KeySync_DoesNotPerformASyncIfSyncStatusIsUpToDateAndSecretExists() {
	entry := &cache.Entry{}
	entry.CurrentKey.JSON = key1.json
	entry.CurrentKey.ID = key1.id

	// pretend cache entry has already been synced for this gsk
	entry.SyncStatus = map[string]string{
		"my-namespace/my-gsk": "515a2a04abd78d13b0df1e4bc0163e1a787439fd968f364794083fa995fed009:" + key1.id,
	}

	gsk := apiv1b1.GCPSaKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gsk",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
		Spec: apiv1b1.GCPSaKeySpec{
			Secret: apiv1b1.Secret{
				Name:        "my-secret",
				PemKeyName:  "my-key.pem",
				JsonKeyName: "my-key.json",
			},
			VaultReplications: []apiv1b1.VaultReplication{},
		},
	}

	// create the gsk's secret - this should prevent the key sync from running, even the secret does not have
	// the key data in it
	suite.createSecret(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "my-namespace",
		},
	})

	suite.cache.EXPECT().Save(entry).Return(nil)

	// run a key sync to create the secret once
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, gsk))

	secret, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	// make sure secret is empty
	assert.Empty(suite.T(), secret.Data)
}

func (suite *KeySyncSuite) Test_KeySync_PrunesOldStatusEntries() {
	entry := &cache.Entry{}
	entry.CurrentKey.JSON = key1.json
	entry.CurrentKey.ID = key1.id
	entry.SyncStatus = map[string]string{
		"my-namespace/my-gsk":         "515a2a04abd78d13b0df1e4bc0163e1a787439fd968f364794083fa995fed009:" + key1.id, // should not be deleted
		"my-namespace/deleted-gsk":    "515a2a04abd78d13b0df1e4bc0163e1a787439fd968f364794083fa995fed009:" + key1.id, // should be deleted
		"other-namespace/deleted-gsk": "515a2a04abd78d13b0df1e4bc0163e1a787439fd968f364794083fa995fed009:" + key1.id, // should be deleted
	}

	gsk := apiv1b1.GCPSaKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gsk",
			Namespace: "my-namespace",
		},
		Spec: apiv1b1.GCPSaKeySpec{
			Secret: apiv1b1.Secret{
				Name:        "my-secret",
				PemKeyName:  "my-key.pem",
				JsonKeyName: "my-key.json",
			},
			VaultReplications: []apiv1b1.VaultReplication{},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)

	// run a key sync
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, gsk))

	// make sure the cache entry's sync status map has exactly one record was updated with correct key-sync records
	assert.Len(suite.T(), entry.SyncStatus, 1) // length should b
	assert.Equal(suite.T(), "515a2a04abd78d13b0df1e4bc0163e1a787439fd968f364794083fa995fed009:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
}

func (suite *KeySyncSuite) assertVaultServerHasSecret(path string, content map[string]interface{}) {
	data := suite.vaultServer.GetSecret(path)
	assert.Equal(suite.T(), content, data)
}

func (suite *KeySyncSuite) getSecret(namespace string, name string) (*corev1.Secret, error) {
	return suite.k8s.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func (suite *KeySyncSuite) createSecret(secret *corev1.Secret) {
	_, err := suite.k8s.CoreV1().Secrets(secret.Namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	require.NoError(suite.T(), err)
}

func (suite *KeySyncSuite) assertK8sSecreDoesNotExist(namespace string, name string) {
	_, err := suite.k8s.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	assert.Error(suite.T(), err)
	assert.True(suite.T(), errors.IsNotFound(err))
}
