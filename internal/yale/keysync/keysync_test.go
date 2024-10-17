package keysync

import (
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"context"
	"encoding/json"
	githubmocks "github.com/broadinstitute/yale/internal/yale/keysync/github/mocks"
	"github.com/broadinstitute/yale/internal/yale/keysync/testutils/gsm"
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
	k8s          kubernetes.Interface
	vaultServer  *vaultutils.FakeVaultServer
	gsmServer    *gsm.FakeGsmServer
	githubClient *githubmocks.Client
	cache        *cachemocks.Cache
	keysync      KeySync
}

func TestKeySyncSuite(t *testing.T) {
	suite.Run(t, new(KeySyncSuite))
}

func (suite *KeySyncSuite) SetupTest() {
	suite.k8s = testutils.NewFakeK8sClient(suite.T())
	suite.vaultServer = vaultutils.NewFakeVaultServer(suite.T())
	suite.gsmServer = gsm.NewFakeGsm(suite.T())
	suite.githubClient = githubmocks.NewClient(suite.T())
	suite.cache = cachemocks.NewCache(suite.T())
	suite.keysync = New(suite.k8s, suite.vaultServer.NewClient(), suite.gsmServer.NewClient(), suite.githubClient, suite.cache)
}

func (suite *KeySyncSuite) TearDownTest() {
	suite.gsmServer.Close()
	suite.gsmServer.AssertExpectations()
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
	assert.Equal(suite.T(), "54dbebdeb257509c0c14a1deb9c089f748a1014d1bd95cdb63934990d9d58d70:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
	assert.Equal(suite.T(), "ac43f2b3c2a67ffdfb7bcdc645a8b77cfec1514f15565a41241bd0dddd91fd6d:"+"1234-1234-1234", entryAcs.SyncStatus["my-namespace/my-acs"])
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
	assert.Equal(suite.T(), "54dbebdeb257509c0c14a1deb9c089f748a1014d1bd95cdb63934990d9d58d70:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
	assert.Equal(suite.T(), "ac43f2b3c2a67ffdfb7bcdc645a8b77cfec1514f15565a41241bd0dddd91fd6d:"+"1234-1234-1234", entryAcs.SyncStatus["my-namespace/my-acs"])
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
	assert.Equal(suite.T(), "2e3c041c321b114ef73418778f45ff32a2ee69a3778ea8c7941ddb7f476caae4:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
	assert.Equal(suite.T(), "e3195092300f9d64d790d1117e8880b85a2a55f6973fbb9f709a9e9e65b693df:"+"1234-1234-1234", entryAcs.SyncStatus["my-namespace/my-acs"])
}

func (suite *KeySyncSuite) Test_KeySync_DoesNotPerformVaultReplicationsIfVaultReplicationIsDisabled() {
	suite.keysync = New(suite.k8s, suite.vaultServer.NewClient(), suite.gsmServer.NewClient(), nil, suite.cache, func(options *Options) {
		options.DisableVaultReplication = true
	})

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
					Path:   "secret/foo/test/json",
					Format: apiv1b1.JSON,
					Key:    "key.json",
				},
			},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)

	// run a key sync to create the K8s secret and perform the vault replications
	gsks := []apiv1b1.GcpSaKey{gsk}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, GcpSaKeysToSyncable(gsks)))

	// verify K8s secret was created
	_, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	// verify Vault replications was not performed
	suite.assertVaultServerHasNoSecretAtPath("secret/foo/test/json")

	assert.Len(suite.T(), entry.SyncStatus, 1)
	assert.Equal(suite.T(), "273df880c058c9a339342a4dcf1cf5f06dedce6f84d0735898ee30e223573260:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
}

func (suite *KeySyncSuite) Test_KeySync_PerformsAllConfiguredGSMReplications() {
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
			GoogleSecretManagerReplications: []apiv1b1.GoogleSecretManagerReplication{
				{
					Format:  apiv1b1.JSON,
					Key:     "",
					Project: "my-project",
					Secret:  "foo-secret-json",
				},
				{
					Format:  apiv1b1.Base64,
					Key:     "",
					Project: "my-project",
					Secret:  "foo-secret-base64",
				},
				{
					Format:  apiv1b1.PEM,
					Key:     "",
					Project: "my-project",
					Secret:  "foo-secret-pem",
				},
				{
					Format:  apiv1b1.JSON,
					Key:     "my-key",
					Project: "my-project",
					Secret:  "foo-secret-json-key",
				},
				{
					Format:  apiv1b1.Base64,
					Key:     "my-key",
					Project: "my-project",
					Secret:  "foo-secret-base64-key",
				},
				{
					Format:  apiv1b1.PEM,
					Key:     "my-key",
					Project: "my-project",
					Secret:  "foo-secret-pem-key",
				},
				{
					Format:  apiv1b1.JSON,
					Key:     "",
					Project: "my-project",
					Secret:  "foo-secret-json-already-exists",
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
			GoogleSecretManagerReplications: []apiv1b1.GoogleSecretManagerReplication{
				{
					Format:  apiv1b1.PlainText,
					Key:     "",
					Project: "my-project",
					Secret:  "acs-secret-plain",
				},
				{
					Format:  apiv1b1.Base64,
					Key:     "",
					Project: "my-project",
					Secret:  "acs-secret-base64",
				},
				{
					Format:  apiv1b1.PlainText,
					Key:     "my-key",
					Project: "my-project",
					Secret:  "acs-secret-plain-key",
				},
				{
					Format:  apiv1b1.Base64,
					Key:     "my-key",
					Project: "my-project",
					Secret:  "acs-secret-base64-key",
				},
			},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)
	suite.cache.EXPECT().Save(entryAcs).Return(nil)

	suite.expectGSMReplication("my-project", "foo-secret-json", []byte(key1.json))
	suite.expectGSMReplication("my-project", "foo-secret-base64", []byte(key1.base64))
	suite.expectGSMReplication("my-project", "foo-secret-pem", []byte(key1.pem))
	suite.expectGSMReplication("my-project", "foo-secret-json-key", suite.wrapJsonKey("my-key", key1.json, true))
	suite.expectGSMReplication("my-project", "foo-secret-base64-key", suite.wrapJsonKey("my-key", key1.base64, false))
	suite.expectGSMReplication("my-project", "foo-secret-pem-key", suite.wrapJsonKey("my-key", key1.pem, false))

	suite.expectGSMReplicationSecretExistsWithCorrectData("my-project", "foo-secret-json-already-exists", []byte(key1.json))

	suite.expectGSMReplication("my-project", "acs-secret-plain", []byte("my-acs-secret"))
	suite.expectGSMReplication("my-project", "acs-secret-base64", []byte("bXktYWNzLXNlY3JldA=="))
	suite.expectGSMReplication("my-project", "acs-secret-plain-key", suite.wrapJsonKey("my-key", "my-acs-secret", false))
	suite.expectGSMReplication("my-project", "acs-secret-base64-key", suite.wrapJsonKey("my-key", "bXktYWNzLXNlY3JldA==", false))

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

	assert.Len(suite.T(), entry.SyncStatus, 1)
	assert.Len(suite.T(), entryAcs.SyncStatus, 1)
	assert.Equal(suite.T(), "4b95e4de60c35435a64bde1fba8a07a3a30de0a8f5d4c75a2580dd10d13083f4:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
	assert.Equal(suite.T(), "538f508d5fc4f0f64bf2e5a01c0c497f9a133cca6afca2e26ecdc06b49204004:"+"1234-1234-1234", entryAcs.SyncStatus["my-namespace/my-acs"])
}

func (suite *KeySyncSuite) Test_KeySync_PerformsExpectedGoogleSAKeyGitHubReplications() {
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
			GitHubReplications: []apiv1b1.GitHubReplication{
				{
					Repo:       "my-org/my-repo",
					Secret:     "MY_SECRET_JSON",
					Format:     apiv1b1.JSON,
					SecretKind: "actions",
				},
				{
					Repo:       "my-org/my-repo",
					Secret:     "MY_SECRET_PEM",
					Format:     apiv1b1.PEM,
					SecretKind: "dependabot",
				},
				{
					Repo:       "my-org/my-repo",
					Secret:     "MY_SECRET_B64",
					Format:     apiv1b1.Base64,
					SecretKind: "actions",
				},
				{
					Repo:       "my-org/my-repo",
					Secret:     "MY_SECRET_PLAIN",
					Format:     apiv1b1.PlainText,
					SecretKind: "dependabot",
				},
			},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)

	suite.githubClient.EXPECT().WriteSecret("my-org", "my-repo", "MY_SECRET_JSON", "actions", []byte(key1.json)).Return(nil)
	suite.githubClient.EXPECT().WriteSecret("my-org", "my-repo", "MY_SECRET_PEM", "dependabot", []byte(key1.pem)).Return(nil)
	suite.githubClient.EXPECT().WriteSecret("my-org", "my-repo", "MY_SECRET_B64", "actions", []byte(key1.base64)).Return(nil)
	suite.githubClient.EXPECT().WriteSecret("my-org", "my-repo", "MY_SECRET_PLAIN", "dependabot", []byte(key1.json)).Return(nil)

	// run a key sync to create the K8s secret and perform the vault replications
	gsks := []apiv1b1.GcpSaKey{gsk}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, GcpSaKeysToSyncable(gsks)))

	// verify K8s secret was created
	_, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	// make sure sync status was generated correctly
	assert.Len(suite.T(), entry.SyncStatus, 1)
	assert.Equal(suite.T(), "e906c9bf32bed8732bda333d568eeb6245988f92a209b3a077d8325b77c12699:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
}

func (suite *KeySyncSuite) Test_KeySync_PerformsExpectedAzureClientSecretGitHubReplications() {
	entry := &cache.Entry{}
	entry.Identifier = cache.AzureClientSecretEntryIdentifier{ApplicationID: "4321-4321-4321", TenantID: "2345-2345-2345"}
	entry.CurrentKey.JSON = "my-acs-secret"
	entry.CurrentKey.ID = "1234-1234-1234"
	entry.Type = cache.AzureClientSecret
	entry.SyncStatus = map[string]string{} // no prior syncs recorded in the map

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
				Name:                "my-secret",
				ClientSecretKeyName: "my-client-secret",
			},
			GitHubReplications: []apiv1b1.GitHubReplication{
				{
					Format:     apiv1b1.PlainText,
					Repo:       "my-org/my-repo",
					Secret:     "MY_SECRET_PLAIN",
					SecretKind: "actions",
				},
				{
					Format:     apiv1b1.Base64,
					Repo:       "my-org/my-repo",
					Secret:     "MY_SECRET_B64",
					SecretKind: "dependabot",
				},
			},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)

	suite.githubClient.EXPECT().WriteSecret("my-org", "my-repo", "MY_SECRET_PLAIN", "actions", []byte("my-acs-secret")).Return(nil)
	suite.githubClient.EXPECT().WriteSecret("my-org", "my-repo", "MY_SECRET_B64", "dependabot", []byte("bXktYWNzLXNlY3JldA==")).Return(nil)

	acsSecrets := []apiv1b1.AzureClientSecret{acs}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, AzureClientSecretsToSyncable(acsSecrets)))

	_, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	assert.Len(suite.T(), entry.SyncStatus, 1)
	assert.Equal(suite.T(), "a176cdedd1fdd294394494789474d4211266e3b00c1ccc9005fc9178cf920350:"+"1234-1234-1234", entry.SyncStatus["my-namespace/my-acs"])
}

func (suite *KeySyncSuite) Test_KeySync_DoesNotPerformGitHubReplicationsIfGitHubReplicationIsDisabled() {
	suite.keysync = New(suite.k8s, suite.vaultServer.NewClient(), suite.gsmServer.NewClient(), nil, suite.cache, func(options *Options) {
		options.DisableGitHubReplication = true
	})

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
			GitHubReplications: []apiv1b1.GitHubReplication{
				{
					Repo:   "my-org/my-repo",
					Secret: "MY_SECRET",
					Format: apiv1b1.JSON,
				},
			},
		},
	}

	suite.cache.EXPECT().Save(entry).Return(nil)

	// run a key sync to create the K8s secret and perform the vault replications
	gsks := []apiv1b1.GcpSaKey{gsk}
	require.NoError(suite.T(), suite.keysync.SyncIfNeeded(entry, GcpSaKeysToSyncable(gsks)))

	// verify K8s secret was created
	_, err := suite.getSecret("my-namespace", "my-secret")
	require.NoError(suite.T(), err)

	// make sure sync status was generated correctly
	assert.Len(suite.T(), entry.SyncStatus, 1)
	assert.Equal(suite.T(), "7b8f2c0978d940d96b59252d1b5adfe888d352e01f6408dac2c59aed1e67903e:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])

	// assert WriteSecret was not called
	suite.githubClient.AssertNotCalled(suite.T(), "WriteSecret")
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
		"my-namespace/my-gsk": "54dbebdeb257509c0c14a1deb9c089f748a1014d1bd95cdb63934990d9d58d70:" + key1.id,
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
		"my-namespace/my-acs": "ac43f2b3c2a67ffdfb7bcdc645a8b77cfec1514f15565a41241bd0dddd91fd6d:1234-1234-1234",
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
		"my-namespace/my-gsk":         "54dbebdeb257509c0c14a1deb9c089f748a1014d1bd95cdb63934990d9d58d70:" + key1.id, // should not be deleted
		"my-namespace/deleted-gsk":    "54dbebdeb257509c0c14a1deb9c089f748a1014d1bd95cdb63934990d9d58d70:" + key1.id, // should be deleted
		"other-namespace/deleted-gsk": "54dbebdeb257509c0c14a1deb9c089f748a1014d1bd95cdb63934990d9d58d70:" + key1.id, // should be deleted
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
	assert.Equal(suite.T(), "54dbebdeb257509c0c14a1deb9c089f748a1014d1bd95cdb63934990d9d58d70:"+key1.id, entry.SyncStatus["my-namespace/my-gsk"])
	assert.Equal(suite.T(), "ac43f2b3c2a67ffdfb7bcdc645a8b77cfec1514f15565a41241bd0dddd91fd6d:1234-1234-1234", entryAcs.SyncStatus["my-namespace/my-acs"])
}

func (suite *KeySyncSuite) expectGSMReplication(project string, secret string, payload []byte) {
	suite.gsmServer.ExpectListSecretWithNameFilter(project, secret, nil)
	suite.gsmServer.ExpectCreateNewSecret(project, secret, func(s *secretmanagerpb.Secret) bool {
		require.Equal(suite.T(), map[string]string{"created-by-yale": "true"}, s.Annotations)
		require.Equal(suite.T(), map[string]string{"owned_by": "yale"}, s.Labels)
		return true
	}, &secretmanagerpb.Secret{
		Name: "ignored",
	})
	suite.gsmServer.ExpectAccessSecretVersion(project, secret, "latest", nil)
	suite.gsmServer.ExpectCreateNewSecretVersion(project, secret, payload, &secretmanagerpb.SecretVersion{
		Name: "ignored",
	})
}

func (suite *KeySyncSuite) expectGSMReplicationSecretExistsWithCorrectData(project string, secret string, payload []byte) {
	suite.gsmServer.ExpectListSecretWithNameFilter(project, secret, &secretmanagerpb.Secret{
		Name: secret,
	})
	suite.gsmServer.ExpectAccessSecretVersion(project, secret, "latest", payload)
}

func (suite *KeySyncSuite) assertVaultServerHasSecret(path string, content map[string]interface{}) {
	data := suite.vaultServer.GetSecret(path)
	assert.Equal(suite.T(), content, data)
}

func (suite *KeySyncSuite) assertVaultServerHasNoSecretAtPath(path string) {
	data := suite.vaultServer.GetSecret(path)
	assert.Nil(suite.T(), data)
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

func (suite *KeySyncSuite) wrapJsonKey(key string, data string, unmarshalToObject bool) []byte {
	var value interface{}

	if unmarshalToObject {
		var parsed map[string]interface{}
		err := json.Unmarshal([]byte(data), &parsed)
		require.NoError(suite.T(), err)
		value = parsed
	} else {
		value = data
	}

	result, err := json.Marshal(map[string]interface{}{
		key: value,
	})
	require.NoError(suite.T(), err)
	return result
}
