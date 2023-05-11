package yale

import (
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/cache"
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/cutoff"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"time"
)

func (m *Yale) rotateKey(entry *cache.Entry, cutoffs cutoff.Cutoffs, gsks []apiv1b1.GCPSaKey) error {
	var err error

	if err = m.issueNewKeyIfNeeded(entry, cutoffs); err != nil {
		return err
	}
	if err = m.keysync.SyncIfNeeded(entry, gsks...); err != nil {
		return err
	}
	if err = m.cache.Save(entry); err != nil {
		return err
	}

	return nil
}

// issueNewKeyIfNeed given cache entry and gsk, checks if the entry's current active key needs to be rotated.
// if a rotation is needed (or the cache entry is new/empty), it issues a new sa key, adds it
// to the cache entry, then saves the updated cache entry to k8s.
func (m *Yale) issueNewKeyIfNeeded(entry *cache.Entry, cutoffs cutoff.Cutoffs) error {
	email := entry.ServiceAccount.Email
	project := entry.ServiceAccount.Project

	if entry.CurrentKey.ID == "" {
		logs.Info.Printf("service account %s: current key is missing from cache entry; will issue a new key", email)
	} else if cutoffs.ShouldRotate(entry.CurrentKey.CreatedAt) {
		logs.Info.Printf("service account %s: rotating current key %s (created at %s; rotation age is %s days)", email, entry.CurrentKey.ID, entry.CurrentKey.CreatedAt, cutoffs.RotateAfterDays())
	} else {
		return nil
	}

	if entry.CurrentKey.ID != "" {
		entry.RotatedKeys[entry.CurrentKey.ID] = time.Now()
	}

	logs.Info.Printf("Creating new service account key for %s", email)
	newKey, json, err := m.keyops.Create(project, email)
	if err != nil {
		return fmt.Errorf("error issuing new service account key for %s: %v", email, err)
	}

	logs.Info.Printf("created new service account key %s for %s", newKey.ID, email)

	entry.CurrentKey.ID = newKey.ID
	entry.CurrentKey.JSON = string(json)
	entry.CurrentKey.CreatedAt = time.Now()

	if err = m.cache.Save(entry); err != nil {
		return fmt.Errorf("error saving cache entry for %s after key rotation: %v", email, err)
	}

	return nil
}
