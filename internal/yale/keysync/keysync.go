package keysync

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/cache"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	vaultapi "github.com/hashicorp/vault/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const defaultVaultReplicationSecretKey = "sa-key"

// KeySync is responsible for propagating the current service account key from the Yale cache to destinations
// specified in the GcpSaKey spec - Vault paths, Kubernetes secrets, etc.
type KeySync interface {
	// SyncIfNeeded for every given gsk, sync the current service account key in the cache entry to
	// the Kubernetes secret and Vault paths that are specified in the gsk's spec.
	//
	// Note that this function will update the cache entry's SyncStatus map to reflect any sync's it performs,
	// but it WILL NOT save the entry to the cache -- that's the caller's responsibility!
	SyncIfNeeded(entry *cache.Entry, gsks ...apiv1b1.GCPSaKey) error
}

func New(k8s kubernetes.Interface, vault *vaultapi.Client) KeySync {
	return &keysync{
		k8s:   k8s,
		vault: vault,
	}
}

type keysync struct {
	vault *vaultapi.Client
	k8s   kubernetes.Interface
}

func (k *keysync) SyncIfNeeded(entry *cache.Entry, gsks ...apiv1b1.GCPSaKey) error {
	for _, gsk := range gsks {
		mapKey := gsk.Namespace + "/" + gsk.Name
		data, err := json.Marshal(gsk.Spec)
		if err != nil {
			return fmt.Errorf("gsk %s in %s: error marshalling gsk spec to JSON: %v", gsk.Name, gsk.Namespace, err)
		}
		checksum, err := sha256Sum(data)
		if err != nil {
			return fmt.Errorf("gsk %s in %s: error computing sha265sum for gsk spec: %v", gsk.Name, gsk.Namespace, err)
		}
		expected := checksum + ":" + entry.CurrentKey.ID
		actual := entry.SyncStatus[mapKey]

		logs.Info.Printf("gsk %s in %s: sync status should be %q, is %q", gsk.Name, gsk.Namespace, expected, actual)
		if actual == expected {
			continue
		}
		logs.Info.Printf("gsk %s in %s: starting key sync", gsk.Name, gsk.Namespace)
		if err = k.syncToK8sSecret(entry, gsk); err != nil {
			return fmt.Errorf("gsk %s in %s: error syncing to K8s secret: %v", gsk.Name, gsk.Namespace, err)
		}
		if err = k.replicateKeyToVault(entry, gsk); err != nil {
			return fmt.Errorf("gsk %s in %s: error syncing to Vault: %v", gsk.Name, gsk.Namespace, err)
		}
		entry.SyncStatus[mapKey] = expected
	}

	return nil
}

func (k *keysync) syncToK8sSecret(entry *cache.Entry, gsk apiv1b1.GCPSaKey) error {
	namespace := gsk.Namespace

	secret, err := k.k8s.CoreV1().Secrets(namespace).Get(context.Background(), gsk.Spec.Secret.Name, metav1.GetOptions{})
	var create bool

	if err != nil {
		if errors.IsNotFound(err) {
			// Create ownership reference
			// https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents
			var ownerRef = []metav1.OwnerReference{
				{
					APIVersion: gsk.APIVersion,
					Kind:       gsk.Kind,
					Name:       gsk.Name,
					UID:        gsk.UID,
				},
			}

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       gsk.Namespace,
					Name:            gsk.Spec.Secret.Name,
					OwnerReferences: ownerRef,
				},
				Type: corev1.SecretTypeOpaque,
			}
			create = true
		} else {
			return fmt.Errorf("gsk %s in %s: error retrieving referenced secret %s: %v", gsk.Name, gsk.Namespace, gsk.Spec.Secret.Name, err)
		}
	}

	// add labels and annotations to the secret if they aren't already there
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	for k, v := range gsk.Labels {
		secret.Labels[k] = v
	}

	// make sure reloader annotations are added to the secret
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	secret.Annotations["reloader.stakater.com/match"] = "true"

	// extract pem-formatted key from the service account key JSON
	pemFormatted, err := extractPemKey(entry)
	if err != nil {
		return fmt.Errorf("gsk %s in %s: error extracting PEM-formatted key for %s: %v", gsk.Name, gsk.Namespace, entry.ServiceAccount.Email, err)
	}

	// add the key data to the secret
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data[gsk.Spec.Secret.JsonKeyName] = []byte(entry.CurrentKey.JSON)
	secret.Data[gsk.Spec.Secret.PemKeyName] = []byte(pemFormatted)

	if create {
		_, err = k.k8s.CoreV1().Secrets(gsk.Namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	} else {
		_, err = k.k8s.CoreV1().Secrets(gsk.Namespace).Update(context.Background(), secret, metav1.UpdateOptions{})
	}
	return err
}

func (k *keysync) replicateKeyToVault(entry *cache.Entry, gsk apiv1b1.GCPSaKey) error {
	for _, spec := range gsk.Spec.VaultReplications {
		msg := fmt.Sprintf("replicating key %s for %s to Vault (format %s, path %s, key %s)",
			entry.CurrentKey.ID, entry.ServiceAccount.Email, spec.Format, spec.Path, spec.Key)
		logs.Info.Print(msg)
		secretData, err := prepareVaultSecret(entry, spec)
		if err != nil {
			return fmt.Errorf("error %s: decoding failed: %v", msg, err)
		}

		if _, err = k.vault.Logical().Write(spec.Path, secretData); err != nil {
			return fmt.Errorf("error %s: write failed: %v", msg, err)
		}
	}
	return nil
}

func prepareVaultSecret(entry *cache.Entry, spec apiv1b1.VaultReplication) (map[string]interface{}, error) {
	asJson := []byte(entry.CurrentKey.JSON)
	base64Encoded := base64.StdEncoding.EncodeToString(asJson)

	asPem, err := extractPemKey(entry)
	if err != nil {
		return nil, err
	}

	secret := make(map[string]interface{})
	secretKey := spec.Key
	if secretKey == "" {
		secretKey = defaultVaultReplicationSecretKey
	}

	switch spec.Format {
	case apiv1b1.Map:
		if err := json.Unmarshal(asJson, &secret); err != nil {
			return nil, fmt.Errorf("error decoding private key to secret map: %v", err)
		}
	case apiv1b1.JSON:
		secret[secretKey] = string(asJson)
	case apiv1b1.Base64:
		secret[secretKey] = base64Encoded
	case apiv1b1.PEM:
		secret[secretKey] = asPem
	default:
		panic(fmt.Errorf("unsupported Vault replication format: %#v", spec.Format))
	}

	return secret, nil
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

// return the PEM-formatted private_key field from a cache entry's JSON-formatted SA key
func extractPemKey(entry *cache.Entry) (string, error) {
	asJson := []byte(entry.CurrentKey.JSON)

	type keyJson struct {
		PrivateKey string `json:"private_key"`
	}
	var k keyJson
	if err := json.Unmarshal(asJson, &k); err != nil {
		return "", fmt.Errorf("failed to decode key %s (%s) from JSON: %v", entry.CurrentKey.ID, entry.ServiceAccount.Email, err)
	}
	return k.PrivateKey, nil
}