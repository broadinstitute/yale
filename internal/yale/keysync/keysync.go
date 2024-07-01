package keysync

import (
	"bytes"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/keysync/github"
	"google.golang.org/api/iterator"
	"strings"
	"sync"

	"github.com/broadinstitute/yale/internal/yale/cache"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	vaultapi "github.com/hashicorp/vault/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const defaultVaultReplicationSecretKey = "sa-key"

type Option func(*Options)

type Options struct {
	DisableVaultReplication  bool
	DisableGitHubReplication bool
}

// KeySync is responsible for propagating the current service account key from the Yale cache to destinations
// specified in the GcpSaKey spec - Vault paths, Kubernetes secrets, etc.
type KeySync interface {
	// SyncIfNeeded for every given gsk, sync the current service account key in the cache entry to
	// the Kubernetes secret and Vault paths that are specified in the gsk's spec.
	//
	// Note that this function will update the cache entry's SyncStatus map to reflect any sync's it performs,
	// but it WILL NOT save the entry to the cache -- that's the caller's responsibility!
	SyncIfNeeded(entry *cache.Entry, gsks []Syncable) error
}

// Syncable is an interface for objects that can be synced to a Kubernetes secret
// It is a collection of getter methods that return the information needed to sync to a k8s
// secret or vault path.
type Syncable interface {
	Name() string
	Namespace() string
	SecretName() string
	Secret() apiv1b1.Secret
	SpecBytes() ([]byte, error)
	VaultReplications() []apiv1b1.VaultReplication
	GoogleSecretManagerReplications() []apiv1b1.GoogleSecretManagerReplication
	GitHubReplications() []apiv1b1.GitHubReplication
	APIVersion() string
	Kind() string
	UID() types.UID
	Labels() map[string]string
}

func GcpSaKeysToSyncable(gsks []apiv1b1.GcpSaKey) []Syncable {
	var result []Syncable
	for _, syncable := range gsks {
		result = append(result, syncable)
	}
	return result
}

func AzureClientSecretsToSyncable(acs []apiv1b1.AzureClientSecret) []Syncable {
	var result []Syncable
	for _, syncable := range acs {
		result = append(result, syncable)
	}
	return result
}

func New(k8s kubernetes.Interface, vault *vaultapi.Client, secretManager *secretmanager.Client, github github.Client, cache cache.Cache, options ...Option) KeySync {
	opts := Options{
		DisableVaultReplication: false,
	}
	for _, option := range options {
		option(&opts)
	}
	return &keysync{
		options:       opts,
		k8s:           k8s,
		vault:         vault,
		secretManager: secretManager,
		github:        github,
		cache:         cache,
	}
}

type keysync struct {
	options        Options
	vault          *vaultapi.Client
	secretManager  *secretmanager.Client
	github         github.Client
	k8s            kubernetes.Interface
	cache          cache.Cache
	mutex          sync.Mutex
	clusterSecrets map[string]struct{}
}

func (k *keysync) SyncIfNeeded(entry *cache.Entry, syncables []Syncable) error {
	for _, syncable := range syncables {
		syncRequired, statusHash, err := k.syncRequired(entry, syncable)
		if err != nil {
			return err
		}
		if !syncRequired {
			continue
		}
		logs.Info.Printf("%s %s in %s: starting key sync", entry.Type, syncable.Name(), syncable.Namespace())
		if err = k.syncToK8sSecret(entry, syncable); err != nil {
			return fmt.Errorf("%s %s in %s: error syncing to K8s secret: %v", entry.Type, syncable.Name(), syncable.Namespace(), err)
		}
		if err = k.replicateKeyToVault(entry, syncable); err != nil {
			return fmt.Errorf("%s %s in %s: error syncing to Vault: %v", entry.Type, syncable.Name(), syncable.Namespace(), err)
		}
		if err = k.replicateKeyToGSM(entry, syncable); err != nil {
			return fmt.Errorf("%s %s in %s: error syncing to GSM: %v", entry.Type, syncable.Name(), syncable.Namespace(), err)
		}
		if err = k.replicateKeyToGitHub(entry, syncable); err != nil {
			return fmt.Errorf("%s %s in %s: error syncing to GitHub: %v", entry.Type, syncable.Name(), syncable.Namespace(), err)
		}
		entry.SyncStatus[statusKey(syncable)] = statusHash
	}

	pruneOldSyncStatuses(entry, syncables...)

	if err := k.cache.Save(entry); err != nil {
		return fmt.Errorf("error saving cache entry for %s after key sync: %v", entry.Identify(), err)
	}

	return nil
}

// syncRequired determine if a gsk needs to be synced from its cache entry to its k8s secret.
// this is true if:
// - the secret does not exist
// - the secret exists, but the gsk's spec has changed since the last sync
// - the secret exists, but the service account key has been rotated since the last sync
//
// note that the latter two conditions are detected by computing the gsk's status hash and comparing
// it to the one stored in the cache entry's status map.
//
// this method also returns the computed status hash, which is used to update the cache entry's SyncStatus map
// after a successful sync
func (k *keysync) syncRequired(entry *cache.Entry, syncable Syncable) (bool, string, error) {
	// compute the statusHash for the gsk
	computedHash, err := computeStatusHash(entry, syncable)
	if err != nil {
		return false, "", err
	}

	// first, check if the secret exists. If it was deleted (eg. manually in the UI),
	// Yale should absolutely perform a sync
	secretExists, err := k.clusterHasSecret(syncable)
	if err != nil {
		return false, "", err
	}
	if !secretExists {
		logs.Info.Printf("%s %s in %s: secret %s does not exist, key sync is needed", entry.Type, syncable.Name(), syncable.Namespace(), syncable.SecretName())
		return true, computedHash, nil
	}

	cachedHash := entry.SyncStatus[statusKey(syncable)]

	logs.Info.Printf("%s %s in %s: sync status should be %q, is %q", entry.Type, syncable.Name(), syncable.Namespace(), computedHash, cachedHash)
	if cachedHash == computedHash {
		return false, computedHash, nil
	}
	return true, computedHash, nil
}

func (k *keysync) syncToK8sSecret(entry *cache.Entry, syncable Syncable) error {
	namespace := syncable.Namespace()

	secret, err := k.k8s.CoreV1().Secrets(namespace).Get(context.Background(), syncable.SecretName(), metav1.GetOptions{})
	var create bool

	if err != nil {
		if errors.IsNotFound(err) {
			// Create ownership reference
			// https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents
			var ownerRef = []metav1.OwnerReference{
				{
					APIVersion: syncable.APIVersion(),
					Kind:       syncable.Kind(),
					Name:       syncable.Name(),
					UID:        syncable.UID(),
				},
			}

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       syncable.Namespace(),
					Name:            syncable.SecretName(),
					OwnerReferences: ownerRef,
				},
				Type: corev1.SecretTypeOpaque,
			}
			create = true
		} else {
			return fmt.Errorf("%s %s in %s: error retrieving referenced secret %s: %v", entry.Type, syncable.Name(), syncable.Namespace(), syncable.SecretName(), err)
		}
	}

	// add labels and annotations to the secret if they aren't already there
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	for k, v := range syncable.Labels() {
		secret.Labels[k] = v
	}

	// make sure reloader annotations are added to the secret
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	secret.Annotations["reloader.stakater.com/match"] = "true"

	// add the key data to the secret
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}

	// extract pem-formatted key from the service account key JSON if dealing with a GCP SA key type
	if entry.Type == cache.GcpSaKey {
		pemFormatted, err := extractPemKey(entry)
		if err != nil {
			return fmt.Errorf("%s %s in %s: error extracting PEM-formatted key for %s: %v", entry.Type, syncable.Name(), syncable.Namespace(), entry.Identify(), err)
		}
		// add the key data to the secret
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		secret.Data[syncable.Secret().JsonKeyName] = []byte(entry.CurrentKey.JSON)
		secret.Data[syncable.Secret().PemKeyName] = []byte(pemFormatted)
	} else if entry.Type == cache.AzureClientSecret {
		secret.Data[syncable.Secret().ClientSecretKeyName] = []byte(entry.CurrentKey.JSON)
	}

	if create {
		_, err = k.k8s.CoreV1().Secrets(syncable.Namespace()).Create(context.Background(), secret, metav1.CreateOptions{})
	} else {
		_, err = k.k8s.CoreV1().Secrets(syncable.Namespace()).Update(context.Background(), secret, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("error syncing %s %s to secret %s/%s: %v", entry.Type, entry.CurrentKey.ID, syncable.Namespace(), secret.Name, err)
	}
	logs.Info.Printf("synced %s %s to secret %s/%s", entry.Type, entry.CurrentKey.ID, syncable.Namespace(), syncable.SecretName())
	return nil
}

func (k *keysync) replicateKeyToVault(entry *cache.Entry, syncable Syncable) error {
	if k.options.DisableVaultReplication {
		return nil
	}

	if len(syncable.VaultReplications()) == 0 {
		// no replications to perform
		return nil
	}

	for _, spec := range syncable.VaultReplications() {
		msg := fmt.Sprintf("replicating key %s for %s to Vault (format %s, path %s, key %s)",
			entry.CurrentKey.ID, entry.Identify(), spec.Format, spec.Path, spec.Key)
		logs.Info.Print(msg)
		secretData, err := prepareVaultSecret(entry, spec)
		if err != nil {
			return fmt.Errorf("error %s: decoding failed: %v", msg, err)
		}

		if _, err = k.vault.Logical().Write(spec.Path, secretData); err != nil {
			return fmt.Errorf("error %s: write failed: %v", msg, err)
		}
	}

	logs.Info.Printf("replicated key %s for %s to %d Vault paths", entry.CurrentKey.ID, entry.Identify(), len(syncable.VaultReplications()))

	return nil
}

func prepareVaultSecret(entry *cache.Entry, spec apiv1b1.VaultReplication) (map[string]interface{}, error) {
	currentKey := []byte(entry.CurrentKey.JSON)
	base64Encoded := base64.StdEncoding.EncodeToString(currentKey)
	var asPem string
	if entry.Type == cache.GcpSaKey {
		var err error
		asPem, err = extractPemKey(entry)
		if err != nil {
			return nil, err
		}
	}

	secret := make(map[string]interface{})
	secretKey := spec.Key
	if secretKey == "" {
		secretKey = defaultVaultReplicationSecretKey
	}

	switch spec.Format {
	case apiv1b1.Map:
		if entry.Type == cache.AzureClientSecret {
			return nil, fmt.Errorf("error decoding client secret to secret map: Azure client secret is not a JSON object. Map type vault replication is only supported for GCP service account keys")
		}
		if err := json.Unmarshal(currentKey, &secret); err != nil {
			return nil, fmt.Errorf("error decoding private key to secret map: %v", err)
		}
	case apiv1b1.JSON:
		// technically should raise an error here for ACS secrets (they aren't JSON) but I don't want
		// to break CRDs that have already been deployed
		secret[secretKey] = string(currentKey)
	case apiv1b1.PlainText:
		secret[secretKey] = string(currentKey)
	case apiv1b1.Base64:
		secret[secretKey] = base64Encoded
	case apiv1b1.PEM:
		if entry.Type == cache.AzureClientSecret {
			return nil, fmt.Errorf("error decoding client secret to PEM: Azure client secret is not a JSON object. PEM type vault replication is only supported for GCP service account keys")
		}
		secret[secretKey] = asPem
	default:
		panic(fmt.Errorf("unsupported Vault replication format: %#v", spec.Format))
	}

	return secret, nil
}

func (k *keysync) replicateKeyToGSM(entry *cache.Entry, syncable Syncable) error {
	if len(syncable.GoogleSecretManagerReplications()) == 0 {
		// no replications to perform
		return nil
	}

	for _, spec := range syncable.GoogleSecretManagerReplications() {
		msg := fmt.Sprintf("replicating key %s for %s (format %s) to GSM (project %s, secret %s)",
			entry.CurrentKey.ID, entry.Identify(), spec.Format, spec.Project, spec.Secret)
		logs.Info.Print(msg)

		secretData, err := prepareGoogleSecretManagerSecret(entry, spec)
		if err != nil {
			return fmt.Errorf("error %s: decoding failed: %v", msg, err)
		}

		itr := k.secretManager.ListSecrets(context.Background(), &secretmanagerpb.ListSecretsRequest{
			Parent: fmt.Sprintf("projects/%s", spec.Project),
			Filter: fmt.Sprintf("name:%s", spec.Secret),
		})

		// there can only be between 0 and 1 secrets that match the filter
		var secrets []*secretmanagerpb.Secret
		for {
			secret, err := itr.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("error searching GSM API for secret %s in project %s: %v", spec.Secret, spec.Project, err)
			}
			secrets = append(secrets, secret)
		}

		if len(secrets) == 0 {
			logs.Info.Printf("found no secret %s in project %s, creating...",
				spec.Secret, spec.Project)

			_, err = k.secretManager.CreateSecret(context.Background(), &secretmanagerpb.CreateSecretRequest{
				Parent:   fmt.Sprintf("projects/%s", spec.Project),
				SecretId: spec.Secret,
				Secret: &secretmanagerpb.Secret{
					Name: spec.Secret,
					Annotations: map[string]string{
						"created-by-yale": "true",
					},
					Labels: map[string]string{
						"owned_by": "yale",
					},
					Replication: &secretmanagerpb.Replication{
						Replication: &secretmanagerpb.Replication_Automatic_{
							Automatic: &secretmanagerpb.Replication_Automatic{},
						},
					},
				},
			})
			if err != nil {
				return fmt.Errorf("error creating new GSM secret %s in project %s: %v", spec.Secret, spec.Project, err)
			}
		}

		logs.Info.Printf("pulling latest GSM secret version for %s in project %s", spec.Secret, spec.Project)
		secretVersion, err := k.secretManager.AccessSecretVersion(context.Background(), &secretmanagerpb.AccessSecretVersionRequest{
			Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", spec.Project, spec.Secret),
		})
		if err != nil {
			logs.Info.Printf("received error pulling latest GSM secret version for %s in %s; assuming secret has no versions: %v", spec.Secret, spec.Project, err)
		} else {
			if bytes.Equal(secretVersion.GetPayload().GetData(), secretData) {
				logs.Info.Printf("GSM secret %s in %s already contains the desired data, won't create a new secret version", spec.Secret, spec.Project)
				continue
			}
		}

		logs.Info.Printf("creating new GSM secret version for %s in project %s", spec.Secret, spec.Project)
		newVersion, err := k.secretManager.AddSecretVersion(context.Background(), &secretmanagerpb.AddSecretVersionRequest{
			Parent: fmt.Sprintf("projects/%s/secrets/%s", spec.Project, spec.Secret),
			Payload: &secretmanagerpb.SecretPayload{
				Data: secretData,
			},
		})
		if err != nil {
			return fmt.Errorf("error creating new GSM secret version for %s in project %s: %v", spec.Secret, spec.Project, err)
		}

		logs.Info.Printf("created new GSM secret version for %s in project %s: %s", spec.Secret, spec.Project, newVersion.Name)
	}

	logs.Info.Printf("replicated key %s for %s to %d GSM secrets", entry.CurrentKey.ID, entry.Identify(), len(syncable.GoogleSecretManagerReplications()))

	return nil
}

func prepareGoogleSecretManagerSecret(entry *cache.Entry, spec apiv1b1.GoogleSecretManagerReplication) ([]byte, error) {
	formattedBytes, err := formatSecretForGitHubOrGSM(entry, spec.Format)
	if err != nil {
		return nil, err
	}
	if spec.Key == "" {
		return formattedBytes, nil
	}

	// if a key was specified with a Map or JSON format, return nested JSON, such as:
	// {
	//   "key-name": { ... }
	// }
	var keyedMap map[string]interface{}

	if spec.Format == apiv1b1.JSON {
		var unmarshalled map[string]interface{}
		if err := json.Unmarshal(formattedBytes, &unmarshalled); err != nil {
			return nil, fmt.Errorf("error unmarshalling GCP key to JSON: %v", err)
		}
		keyedMap = map[string]interface{}{
			spec.Key: unmarshalled,
		}
	} else {
		keyedMap = map[string]interface{}{
			spec.Key: string(formattedBytes),
		}
	}

	keyedMapJSON, err := json.Marshal(keyedMap)
	if err != nil {
		return nil, fmt.Errorf("error marshalling GSM secret to JSON: %v", err)
	}
	return keyedMapJSON, nil
}

func (k *keysync) replicateKeyToGitHub(entry *cache.Entry, syncable Syncable) error {
	if k.options.DisableGitHubReplication {
		return nil
	}

	for _, r := range syncable.GitHubReplications() {
		tokens := strings.SplitN(r.Repo, "/", 2)
		if tokens[0] == "" || tokens[1] == "" {
			return fmt.Errorf("invalid repository specified in %s/%s, expected format \"<org>/<repo>\", got: %q", syncable.Namespace(), syncable.Name(), r.Repo)
		}

		org := tokens[0]
		repo := tokens[1]

		formatted, err := formatSecretForGitHubOrGSM(entry, r.Format)
		if err != nil {
			return fmt.Errorf("%s/%s: error formatting secret for %s/%s: %v", syncable.Namespace(), syncable.Name(), org, repo, err)
		}

		logs.Info.Printf("Writing secret for %s/%s to GitHub secret %s in repo %s (format: %s)", syncable.Namespace(), syncable.Name(), r.Secret, r.Repo, r.Format)

		err = k.github.WriteSecret(org, repo, r.Secret, formatted)
		if err != nil {
			return fmt.Errorf("%s/%s: error writing GitHub secret %s in repo %s/%s: %v", syncable.Namespace(), syncable.Name(), r.Secret, org, repo, err)
		}
	}

	return nil
}

func formatSecretForGitHubOrGSM(entry *cache.Entry, format apiv1b1.ReplicationFormat) ([]byte, error) {
	asJSONString := entry.CurrentKey.JSON
	asJSONBytes := []byte(asJSONString)
	var asPem string
	if entry.Type == cache.GcpSaKey {
		var err error
		asPem, err = extractPemKey(entry)
		if err != nil {
			return nil, err
		}
	}

	var encodedValue string

	switch format {
	case apiv1b1.Map:
		return nil, fmt.Errorf("map format is not supported for GSM or GitHub replications")
	case apiv1b1.JSON:
		if entry.Type == cache.AzureClientSecret {
			return nil, fmt.Errorf("error decoding client secret to secret map: Azure client secret is not a JSON object. JSON type replication is only supported for GCP service account keys")
		}
		encodedValue = asJSONString
	case apiv1b1.PlainText:
		encodedValue = asJSONString
	case apiv1b1.Base64:
		encodedValue = base64.StdEncoding.EncodeToString(asJSONBytes)
	case apiv1b1.PEM:
		if entry.Type == cache.AzureClientSecret {
			return nil, fmt.Errorf("error decoding client secret to PEM: Azure client secret is not a JSON object. PEM type vault replication is only supported for GCP service account keys")
		}
		encodedValue = asPem
	default:
		panic(fmt.Errorf("unsupported replication format for GSM and GitHub: %#v", format.String()))
	}

	return []byte(encodedValue), nil
}

// return the PEM-formatted private_key field from a cache entry's JSON-formatted SA key
func extractPemKey(entry *cache.Entry) (string, error) {
	asJson := []byte(entry.CurrentKey.JSON)

	type keyJson struct {
		PrivateKey string `json:"private_key"`
	}
	var k keyJson
	if err := json.Unmarshal(asJson, &k); err != nil {
		return "", fmt.Errorf("failed to decode key %s (%s) from JSON: %v", entry.CurrentKey.ID, entry.Identify(), err)
	}
	return k.PrivateKey, nil
}

// prune references to old gsks that no longer exists from the sync status map
// We do this because K8s imposes a size limit of 1mb on secrets, and in
// BEE clusters new BEEs with unique names are constantly being created and deleted
func pruneOldSyncStatuses(entry *cache.Entry, syncables ...Syncable) {
	keepKeys := make(map[string]struct{})

	// build a map of keys for gsks that currently exist in the cluster
	for _, syncable := range syncables {
		key := statusKey(syncable)
		keepKeys[key] = struct{}{}
	}

	// prune old
	for key := range entry.SyncStatus {
		_, exists := keepKeys[key]
		if !exists {
			delete(entry.SyncStatus, key)
		}
	}
}

// compute the expected status map value for a given gsk, which is the sha256 checksum
// of the gsk's spec, concatenated with the ID of the cache entry's current service account key
// eg. "<sha-256-sum>:<key-id>"
func computeStatusHash(entry *cache.Entry, syncable Syncable) (string, error) {
	data, err := syncable.SpecBytes()
	if err != nil {
		return "", fmt.Errorf("%s %s in %s: error marshalling gsk spec to JSON: %v", entry.Type, syncable.Name(), syncable.Namespace(), err)
	}
	checksum, err := sha256Sum(data)
	if err != nil {
		return "", fmt.Errorf("%s %s in %s: error computing sha265sum for gsk spec: %v", entry.Type, syncable.Name(), syncable.Namespace(), err)
	}
	return checksum + ":" + entry.CurrentKey.ID, nil
}

// compute a sha256 checksum and return in hex string form, eg.
// "b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"
func sha256Sum(data []byte) (string, error) {
	hash := sha256.New()
	if _, err := hash.Write(data); err != nil {
		return "", fmt.Errorf("error computing checksum: %v", err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// return the key for a gsk in the sync status map
// eg. "<namespace>/<name>"
func statusKey(syncable Syncable) string {
	return qualifiedName(syncable.Namespace(), syncable.Name())
}

// return the key for a secret in the secrets map in the form "<namespace>/<name>"
func secretKeyForGsk(syncable Syncable) string {
	return qualifiedName(syncable.Namespace(), syncable.SecretName())
}

// return the key for a secret in the secrets map in the form "<namespace>/<name>"
func secretKey(secret corev1.Secret) string {
	return qualifiedName(secret.Namespace, secret.Name)
}

// return a qualified name for a k8s resource in the form "<namespace>/<name>"
func qualifiedName(namespace string, name string) string {
	return namespace + "/" + name
}

// clusterHasSecret returns true if the secret specified in the gsk's secret spec
// exists in the cluster, false otherwise
func (k *keysync) clusterHasSecret(syncable Syncable) (bool, error) {
	secrets, err := k.getClusterSecrets()
	if err != nil {
		return false, err
	}
	_, exists := secrets[secretKeyForGsk(syncable)]
	return exists, nil
}

// getClusterSecrets memoized method that returns a set of the names of all secrets in the cluster,
// as a map with keys in the form "<namespace>/<name>"
func (k *keysync) getClusterSecrets() (map[string]struct{}, error) {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	if k.clusterSecrets != nil {
		return k.clusterSecrets, nil
	}

	// we intentionally use `""` for the namespace here, because we want to list all secrets in all namespaces
	list, err := k.k8s.CoreV1().Secrets("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("keysync: error listing secrets in cluster: %v", err)
	}

	m := make(map[string]struct{})
	for _, secret := range list.Items {
		m[secretKey(secret)] = struct{}{}
	}
	k.clusterSecrets = m

	return m, nil
}
