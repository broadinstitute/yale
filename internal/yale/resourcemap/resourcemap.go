package resourcemap

import (
	"context"
	"fmt"

	"github.com/broadinstitute/yale/internal/yale/cache"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	v1beta1client "github.com/broadinstitute/yale/internal/yale/crd/clientset/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Bundle represents a bundle of resources associated with a specific service account
type Bundle struct {
	Entry *cache.Entry
	GSKs  []v1beta1.GcpSaKey
}

// Mapper inspects all the GcpSaKeys and Cache entries in the cluster and organizes
// them into a map[string]*Bundle, where the key is the service account email and
// value is a bundle of all GcpSaKeys associated with that service account, as well
// as its cache entry.
type Mapper interface {
	// Build inspects all the GcpSaKeys and Cache entries in the cluster and organizes
	// them into a map[string]*Bundle, where the key is the service account email and
	// value is a bundle of all GcpSaKeys associated with that service account, as well
	// as its cache entry.
	// If the cluster contains invalid data for a given service account
	// (say, different GcpSaKeys and/or the cache entry reference different projects),
	// BuildMap will log a warning and exclude the service account from the resulting map.
	Build() (map[string]*Bundle, error)
}

func New(crd v1beta1client.YaleCRDInterface, cache cache.Cache) Mapper {
	return &mapper{crd, cache}
}

type mapper struct {
	crd   v1beta1client.YaleCRDInterface
	cache cache.Cache
}

func (m *mapper) Build() (map[string]*Bundle, error) {
	result := make(map[string]*Bundle)

	// list GSKs and organize them into bundles, by service account email
	list, err := m.listGcpSaKeys()
	if err != nil {
		return nil, err
	}

	for _, gsk := range list {
		email := gsk.Spec.GoogleServiceAccount.Name

		bundle, exists := result[email]
		if !exists {
			bundle = &Bundle{}
			result[email] = bundle
		}

		bundle.GSKs = append(bundle.GSKs, gsk)
	}

	// add cache entries to the bundle
	cacheEntries, err := m.cache.List()
	if err != nil {
		return nil, fmt.Errorf("error listing cache entries: %v", err)
	}
	for _, entry := range cacheEntries {
		email := entry.ServiceAccount.Email
		bundle, exists := result[email]
		if !exists {
			bundle = &Bundle{}
			result[email] = bundle
		}
		bundle.Entry = entry
	}

	// filter invalid bundles
	for email, bundle := range result {
		if err = validateResourceBundle(bundle); err != nil {
			logs.Warn.Printf("invalid cluster resources for service account %s, won't process: %v", email, err)
			delete(result, email)
		}
	}

	// add new empty cache entries for any bundles that don't have one
	for email, bundle := range result {
		if bundle.Entry == nil {
			entry, err := m.cache.GetOrCreate(cache.ServiceAccount{
				Email:   email,
				Project: bundle.GSKs[0].Spec.GoogleServiceAccount.Project,
			})
			if err != nil {
				return nil, fmt.Errorf("error creating new empty cache entry for service account %s: %v", email, err)
			}
			bundle.Entry = entry
		}
	}

	return result, nil
}

// listGcpSaKeys retrieves a list of GcpSaKey resources in the cluster, discarding any invalid ones
func (m *mapper) listGcpSaKeys() ([]v1beta1.GcpSaKey, error) {
	list, err := m.crd.GcpSaKeys().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error retrieving list of Yale CRDs from cluster: %v", err)
	}

	var result []v1beta1.GcpSaKey

	for _, gsk := range list.Items {
		if gsk.Spec.GoogleServiceAccount.Name == "" {
			logs.Warn.Printf("GcpSaKey resource %s/%s has invalid spec: missing google service account name", gsk.Namespace, gsk.Name)
			continue
		}
		if gsk.Spec.GoogleServiceAccount.Project == "" {
			logs.Warn.Printf("GcpSaKey resource %s/%s has invalid spec: missing google service account project", gsk.Namespace, gsk.Name)
			continue
		}
		result = append(result, gsk)
	}

	return result, nil
}

// validateResourceBundle verifies that the GcpSaKeys and cache entry in the bundle don't conflict with each other
func validateResourceBundle(bundle *Bundle) error {
	// we have no GSKs, so no need to check if GSKs don't match each other or the cache entry
	if len(bundle.GSKs) == 0 {
		return nil
	}

	// we have at least one GSK - use first as "source of truth" for comparison with other resources
	cmp := bundle.GSKs[0]

	// we have at least 2 GSKs, make sure they all match each other
	if len(bundle.GSKs) > 1 {
		for _, gsk := range bundle.GSKs {
			if gsk.Spec.GoogleServiceAccount.Project != cmp.Spec.GoogleServiceAccount.Project {
				return fmt.Errorf("project mismatch: GcpSaKey resource %s/%s for %s has invalid spec: project %s does not match %s/%s project %s",
					gsk.Namespace, gsk.Name, gsk.Spec.GoogleServiceAccount.Name, gsk.Spec.GoogleServiceAccount.Project,
					cmp.Namespace, cmp.Name, cmp.Spec.GoogleServiceAccount.Project)
			}
		}
	}

	if bundle.Entry == nil {
		// no cache entry to validate
		return nil
	}

	// make sure cache entry has same project as GSK(s)
	if bundle.Entry.ServiceAccount.Project != cmp.Spec.GoogleServiceAccount.Project {
		return fmt.Errorf("project mismatch: cache entry for service account %s has project %s, but GcpSaKey resources like %s/%s have project %s",
			bundle.Entry.ServiceAccount.Email, bundle.Entry.ServiceAccount.Project,
			cmp.Namespace, cmp.Name, cmp.Spec.GoogleServiceAccount.Project)
	}
	return nil
}
