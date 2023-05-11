package yale

import (
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/authmetrics"
	"github.com/broadinstitute/yale/internal/yale/cache"
	"github.com/broadinstitute/yale/internal/yale/client"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/cutoff"
	"github.com/broadinstitute/yale/internal/yale/keyops"
	"github.com/broadinstitute/yale/internal/yale/keysync"
	"github.com/broadinstitute/yale/internal/yale/resourcemap"
)

type Yale struct { // Yale config
	options     Options
	authmetrics authmetrics.AuthMetrics
	keyops      keyops.Keyops
	keysync     keysync.KeySync
	cache       cache.Cache
	resourcemap resourcemap.Mapper
}

type Options struct {
	CacheNamespace string
}

// NewYale /* Construct a new Yale Manager */
func NewYale(clients *client.Clients, opts ...func(*Options)) (*Yale, error) {
	options := Options{
		CacheNamespace: cache.DefaultCacheNamespace,
	}
	for _, opt := range opts {
		opt(&options)
	}

	k8s := clients.GetK8s()
	iam := clients.GetGCP()
	crd := clients.GetCRDs()
	_authmetrics := authmetrics.New(clients.GetMetrics(), iam)
	_keyops := keyops.New(iam)
	_keysync := keysync.New(k8s, clients.GetVault())
	_cache := cache.New(k8s, options.CacheNamespace)
	_resourcemap := resourcemap.New(crd, _cache)

	return &Yale{options, _authmetrics, _keyops, _keysync, _cache, _resourcemap}, nil
}

func (m *Yale) Run() error {
	resources, err := m.resourcemap.Build()
	if err != nil {
		return fmt.Errorf("error inspecting cluster for cache entries and GcpSaKey resources: %v", err)
	}

	for email, bundle := range resources {
		if err = m.processServiceAccount(email, bundle.Entry, bundle.GSKs); err != nil {
			return err
		}
	}
	return nil
}

func (m *Yale) processServiceAccount(email string, entry *cache.Entry, gsks []v1beta1.GCPSaKey) error {
	var err error

	cutoffs := cutoff.New(gsks...)

	if err = m.rotateKey(entry, cutoffs, gsks); err != nil {
		return err
	}

	if err = m.disableOldKeys(entry, cutoffs); err != nil {
		return err
	}
	if err = m.deleteOldKeys(entry, cutoffs); err != nil {
		return err
	}

	return nil
}
