package keysync

import (
	"context"
	"testing"

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
	entry.Type = cache.GcpSaKey
	entry.SyncStatus = map[string]string{} // no prior syncs recorded in the map

	gsk := apiv1b1.GcpSaKey{
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

	entryAcs := &cache.Entry{}
	entryAcs.CurrentKey.JSON = "my-acs-secret"
	entryAcs.CurrentKey.ID = "1234-1234-1234"
	entryAcs.Type = cache.AzureClientSecret
	entryAcs.SyncStatus = map[string]string{} // no prior syncs recorded in the map

	acs := apiv1b1.AzureClientSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-acs",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
		Spec: apiv1b1.AzureClientSecretSpec{
			Secret: apiv1b1.Secret{
				Name:                "my-acs-secret",
				ClientSecretKeyName: "my-client-secret",
			},
			VaultReplications: []apiv1b1.VaultReplication{},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)
	suite.cache.EXPECT().Save(entryAcs).Return(nil)

	suite.assertK8sSecreDoesNotExist("my-namespace", "my-secret")
	suite.assertK8sSecreDoesNotExist("my-namespace", "my-acs-secret")

	// run a key sync
	gsks := []apiv1b1.GcpSaKey{gsk}
	acss := []apiv1b1.AzureClientSecret{acs}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, GcpSaKeysToSyncable(gsks)))
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entryAcs, AzureClientSecretsToSyncable(acss)))

	secret, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	acsSecret, err := suite.getSecret("my-namespace", "my-acs-secret")
	require.NoError(suite.T(), err)

	// make sure secret has the correct ownership reference
	assert.Equal(suite.T(), "my-gsk", secret.OwnerReferences[0].Name)
	assert.Equal(suite.T(), "my-acs", acsSecret.OwnerReferences[0].Name)

	// make sure secret inherited labels from gsk
	assert.Equal(suite.T(), map[string]string{
		"label1": "value1",
		"label2": "value2",
	}, secret.Labels)

	assert.Equal(suite.T(), map[string]string{
		"label1": "value1",
		"label2": "value2",
	}, acsSecret.Labels)

	// make sure secret has reloader annotations
	assert.Equal(suite.T(), "true", secret.Annotations["reloader.stakater.com/match"])
	assert.Equal(suite.T(), "true", acsSecret.Annotations["reloader.stakater.com/match"])

	// make sure secret has expected data
	assert.Equal(suite.T(), key1.json, string(secret.Data["my-key.json"]))
	assert.Equal(suite.T(), key1.pem, string(secret.Data["my-key.pem"]))

	assert.Equal(suite.T(), "my-acs-secret", string(acsSecret.Data["my-client-secret"]))

	// make sure the cache entry was updated with correct key-sync record
	assert.Len(suite.T(), entry.SyncStatus, 1)
	assert.Len(suite.T(), entryAcs.SyncStatus, 1)
	assert.Equal(suite.T(), "bcb8be041cfe2fc4da92ced123f56cb2cc1d6eeb10175d2b4e4348a16c2c235f:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
	assert.Equal(suite.T(), "58df451af5bd0c6b57281b971ff2d7253a70ddeaa62459536135511084aee462:"+"1234-1234-1234", entryAcs.SyncStatus["my-namespace/my-acs"])
}

func (suite *KeySyncSuite) Test_KeySync_UpdatesK8sSecretIfAlreadyExists() {
	entry := &cache.Entry{}
	entry.CurrentKey.JSON = key1.json
	entry.CurrentKey.ID = key1.id
	entry.Type = cache.GcpSaKey
	entry.SyncStatus = map[string]string{} // no prior syncs recorded in the map

	gsk := apiv1b1.GcpSaKey{
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

	entryAcs := &cache.Entry{}
	entryAcs.CurrentKey.JSON = "my-acs-secret"
	entryAcs.CurrentKey.ID = "1234-1234-1234"
	entryAcs.Type = cache.AzureClientSecret
	entryAcs.SyncStatus = map[string]string{} // no prior syncs recorded in the map

	acs := apiv1b1.AzureClientSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-acs",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
		Spec: apiv1b1.AzureClientSecretSpec{
			Secret: apiv1b1.Secret{
				Name:                "my-acs-secret",
				ClientSecretKeyName: "my-client-secret",
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

	suite.createSecret(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-acs-secret",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1":      "this should be overwritten",
				"extra-label": "this should be ignored",
			},
		},
		Data: map[string][]byte{
			"my-client-secret": []byte("this should be overwritten"),
			"extra-data":       []byte("this should be ignored"),
		},
	})

	suite.cache.EXPECT().Save(entry).Return(nil)
	suite.cache.EXPECT().Save(entryAcs).Return(nil)

	// run a key sync to create the secret once
	gsks := []apiv1b1.GcpSaKey{gsk}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, GcpSaKeysToSyncable(gsks)))
	acss := []apiv1b1.AzureClientSecret{acs}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entryAcs, AzureClientSecretsToSyncable(acss)))

	secret, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	acsSecret, err := suite.getSecret("my-namespace", "my-acs-secret")
	require.NoError(suite.T(), err)

	// make sure secret inherited labels from gsk
	assert.Equal(suite.T(), map[string]string{
		"label1":      "value1",
		"label2":      "value2",
		"extra-label": "this should be ignored",
	}, secret.Labels)

	assert.Equal(suite.T(), map[string]string{
		"label1":      "value1",
		"label2":      "value2",
		"extra-label": "this should be ignored",
	}, acsSecret.Labels)

	// make sure secret has reloader annotations
	assert.Equal(suite.T(), "true", secret.Annotations["reloader.stakater.com/match"])
	assert.Equal(suite.T(), "true", acsSecret.Annotations["reloader.stakater.com/match"])

	// make sure secret has expected data
	assert.Equal(suite.T(), key1.json, string(secret.Data["my-key.json"]))
	assert.Equal(suite.T(), key1.pem, string(secret.Data["my-key.pem"]))
	assert.Equal(suite.T(), "this should be ignored", string(secret.Data["extra-data"]))

	assert.Equal(suite.T(), "my-acs-secret", string(acsSecret.Data["my-client-secret"]))
	assert.Equal(suite.T(), "this should be ignored", string(acsSecret.Data["extra-data"]))

	// make sure the cache entry was updated with correct key-sync record
	assert.Len(suite.T(), entry.SyncStatus, 1)
	assert.Len(suite.T(), entryAcs.SyncStatus, 1)
	assert.Equal(suite.T(), "bcb8be041cfe2fc4da92ced123f56cb2cc1d6eeb10175d2b4e4348a16c2c235f:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
	assert.Equal(suite.T(), "58df451af5bd0c6b57281b971ff2d7253a70ddeaa62459536135511084aee462:"+"1234-1234-1234", entryAcs.SyncStatus["my-namespace/my-acs"])
}

func (suite *KeySyncSuite) Test_KeySync_PerformsAllConfiguredVaultReplications() {
	entry := &cache.Entry{}
	entry.Identifier = cache.GcpSaKeyEntryIdentifier{Email: "my-sa@gserviceaccount.com", Project: "my-project"}
	entry.Type = cache.GcpSaKey
	entry.CurrentKey.JSON = key1.json
	entry.CurrentKey.ID = key1.id
	entry.SyncStatus = map[string]string{} // no prior syncs recorded in the map

	gsk := apiv1b1.GcpSaKey{
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

	entryAcs := &cache.Entry{}
	entryAcs.Identifier = cache.AzureClientSecretEntryIdentifier{ApplicationID: "4321-4321-4321", TenantID: "2345-2345-2345"}
	entryAcs.CurrentKey.JSON = "my-acs-secret"
	entryAcs.CurrentKey.ID = "1234-1234-1234"
	entryAcs.Type = cache.AzureClientSecret
	entryAcs.SyncStatus = map[string]string{} // no prior syncs recorded in the map

	acs := apiv1b1.AzureClientSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-acs",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
		Spec: apiv1b1.AzureClientSecretSpec{
			Secret: apiv1b1.Secret{
				Name:                "my-acs-secret",
				ClientSecretKeyName: "my-client-secret",
			},
			VaultReplications: []apiv1b1.VaultReplication{
				{
					Path:   "secret/az/test/json",
					Format: apiv1b1.JSON,
					Key:    "key.json",
				},
				{
					Path:   "secret/az/test/base64",
					Format: apiv1b1.Base64,
					Key:    "key.b64",
				},
			},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)
	suite.cache.EXPECT().Save(entryAcs).Return(nil)

	// run a key sync to create the K8s secret and perform the vault replications
	gsks := []apiv1b1.GcpSaKey{gsk}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, GcpSaKeysToSyncable(gsks)))
	acsSecrets := []apiv1b1.AzureClientSecret{acs}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entryAcs, AzureClientSecretsToSyncable(acsSecrets)))

	// verify K8s secrets were created
	_, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	_, err = suite.getSecret("my-namespace", "my-acs-secret")
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
	suite.assertVaultServerHasSecret("secret/az/test/json", map[string]interface{}{
		"key.json": "my-acs-secret",
	})
	suite.assertVaultServerHasSecret("secret/az/test/base64", map[string]interface{}{
		"key.b64": "bXktYWNzLXNlY3JldA==",
	})
	assert.Len(suite.T(), entry.SyncStatus, 1)
	assert.Len(suite.T(), entryAcs.SyncStatus, 1)
	assert.Equal(suite.T(), "729c209216257d3d2651002acbac6131be54431d6e9914e58821187262e389f8:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
	assert.Equal(suite.T(), "b7af5875fc0db13a4127236649686addf0a86e71c4aee0dbbf11284f78402410:"+"1234-1234-1234", entryAcs.SyncStatus["my-namespace/my-acs"])
}

func (suite *KeySyncSuite) Test_KeySync_PerformsASyncIfSyncStatusIsUpToDateButSecretIsMissing() {
	entry := &cache.Entry{}
	entry.CurrentKey.JSON = key1.json
	entry.CurrentKey.ID = key1.id
	entry.Type = cache.GcpSaKey
	// pretend cache entry has already been synced for this gsk
	entry.SyncStatus = map[string]string{
		"my-namespace/my-gsk": "515a2a04abd78d13b0df1e4bc0163e1a787439fd968f364794083fa995fed009:" + key1.id,
	}

	gsk := apiv1b1.GcpSaKey{
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

	entryAcs := &cache.Entry{}
	entryAcs.CurrentKey.JSON = "my-acs-secret"
	entryAcs.CurrentKey.ID = "1234-1234-1234"
	entryAcs.Type = cache.AzureClientSecret
	entryAcs.SyncStatus = map[string]string{} // no prior syncs recorded in the map

	acs := apiv1b1.AzureClientSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-acs",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
		Spec: apiv1b1.AzureClientSecretSpec{
			Secret: apiv1b1.Secret{
				Name:                "my-acs-secret",
				ClientSecretKeyName: "my-client-secret",
			},
			VaultReplications: []apiv1b1.VaultReplication{},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)
	suite.cache.EXPECT().Save(entryAcs).Return(nil)

	// run a key sync to create the secret once
	gsks := []apiv1b1.GcpSaKey{gsk}
	acss := []apiv1b1.AzureClientSecret{acs}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, GcpSaKeysToSyncable(gsks)))
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entryAcs, AzureClientSecretsToSyncable(acss)))

	secret, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	acsSecret, err := suite.getSecret("my-namespace", "my-acs-secret")
	suite.Require().NoError(err)

	// make sure secret has expected data
	assert.Equal(suite.T(), key1.json, string(secret.Data["my-key.json"]))
	assert.Equal(suite.T(), key1.pem, string(secret.Data["my-key.pem"]))

	suite.Assert().Equal("my-acs-secret", string(acsSecret.Data["my-client-secret"]))
}

func (suite *KeySyncSuite) Test_KeySync_DoesNotPerformASyncIfSyncStatusIsUpToDateAndSecretExists() {
	entry := &cache.Entry{}
	entry.CurrentKey.JSON = key1.json
	entry.CurrentKey.ID = key1.id
	entry.Type = cache.GcpSaKey
	// pretend cache entry has already been synced for this gsk
	entry.SyncStatus = map[string]string{
		"my-namespace/my-gsk": "bcb8be041cfe2fc4da92ced123f56cb2cc1d6eeb10175d2b4e4348a16c2c235f:" + key1.id,
	}

	gsk := apiv1b1.GcpSaKey{
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

	entryAcs := &cache.Entry{}
	entryAcs.CurrentKey.JSON = "my-acs-secret"
	entryAcs.CurrentKey.ID = "1234-1234-1234"
	entryAcs.Type = cache.AzureClientSecret
	entryAcs.SyncStatus = map[string]string{
		"my-namespace/my-acs": "58df451af5bd0c6b57281b971ff2d7253a70ddeaa62459536135511084aee462:1234-1234-1234",
	}

	acs := apiv1b1.AzureClientSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-acs",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
		Spec: apiv1b1.AzureClientSecretSpec{
			Secret: apiv1b1.Secret{
				Name:                "my-acs-secret",
				ClientSecretKeyName: "my-client-secret",
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

	suite.createSecret(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-acs-secret",
			Namespace: "my-namespace",
		},
	})

	suite.cache.EXPECT().Save(entry).Return(nil)
	suite.cache.EXPECT().Save(entryAcs).Return(nil)

	// run a key sync to create the secret once
	gsks := []apiv1b1.GcpSaKey{gsk}
	acss := []apiv1b1.AzureClientSecret{acs}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, GcpSaKeysToSyncable(gsks)))
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entryAcs, AzureClientSecretsToSyncable(acss)))

	secret, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	acsSecret, err := suite.getSecret("my-namespace", "my-acs-secret")
	suite.Require().NoError(err)

	// make sure secret is empty
	assert.Empty(suite.T(), secret.Data)
	suite.Assert().Empty(acsSecret.Data)
}

func (suite *KeySyncSuite) Test_KeySync_PrunesOldStatusEntries() {
	entry := &cache.Entry{}
	entry.CurrentKey.JSON = key1.json
	entry.CurrentKey.ID = key1.id
	entry.Type = cache.GcpSaKey
	entry.SyncStatus = map[string]string{
		"my-namespace/my-gsk":         "bcb8be041cfe2fc4da92ced123f56cb2cc1d6eeb10175d2b4e4348a16c2c235f:" + key1.id, // should not be deleted
		"my-namespace/deleted-gsk":    "bcb8be041cfe2fc4da92ced123f56cb2cc1d6eeb10175d2b4e4348a16c2c235f:" + key1.id, // should be deleted
		"other-namespace/deleted-gsk": "bcb8be041cfe2fc4da92ced123f56cb2cc1d6eeb10175d2b4e4348a16c2c235f:" + key1.id, // should be deleted
	}

	gsk := apiv1b1.GcpSaKey{
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

	entryAcs := &cache.Entry{}
	entryAcs.CurrentKey.JSON = "my-acs-secret"
	entryAcs.CurrentKey.ID = "1234-1234-1234"
	entryAcs.Type = cache.AzureClientSecret
	entryAcs.SyncStatus = map[string]string{
		"my-namespace/my-acs":         "58df451af5bd0c6b57281b971ff2d7253a70ddeaa62459536135511084aee462:1234-1234-1234", // should not be deleted
		"my-namespace/deleted-acs":    "58df451af5bd0c6b57281b971ff2d7253a70ddeaa62459536135511084aee462:1234-1234-1234", // should be deleted
		"other-namespace/deleted-acs": "58df451af5bd0c6b57281b971ff2d7253a70ddeaa62459536135511084aee462:1234-1234-1234", // should be deleted
	}

	acs := apiv1b1.AzureClientSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-acs",
			Namespace: "my-namespace",
			Labels: map[string]string{
				"label1": "value1",
				"label2": "value2",
			},
		},
		Spec: apiv1b1.AzureClientSecretSpec{
			Secret: apiv1b1.Secret{
				Name:                "my-acs-secret",
				ClientSecretKeyName: "my-client-secret",
			},
			VaultReplications: []apiv1b1.VaultReplication{},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)
	suite.cache.EXPECT().Save(entryAcs).Return(nil)

	// run a key sync
	gsks := []apiv1b1.GcpSaKey{gsk}
	acss := []apiv1b1.AzureClientSecret{acs}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, GcpSaKeysToSyncable(gsks)))
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entryAcs, AzureClientSecretsToSyncable(acss)))

	// make sure the cache entry's sync status map has exactly one record was updated with correct key-sync records
	assert.Len(suite.T(), entry.SyncStatus, 1) // length should b
	assert.Len(suite.T(), entryAcs.SyncStatus, 1)
	assert.Equal(suite.T(), "bcb8be041cfe2fc4da92ced123f56cb2cc1d6eeb10175d2b4e4348a16c2c235f:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
	assert.Equal(suite.T(), "58df451af5bd0c6b57281b971ff2d7253a70ddeaa62459536135511084aee462:1234-1234-1234", entryAcs.SyncStatus["my-namespace/my-acs"])
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
