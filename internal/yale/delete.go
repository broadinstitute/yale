package yale

import (
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/cache"
	"github.com/broadinstitute/yale/internal/yale/cutoff"
	"github.com/broadinstitute/yale/internal/yale/keyops"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"time"
)

// deleteOldKeys will delete old service account keys
func (m *Yale) deleteOldKeys(entry *cache.Entry, cutoffs cutoff.Cutoffs) error {
	for keyId, disabledAt := range entry.DisabledKeys {
		if err := m.deleteKey(keyId, disabledAt, entry, cutoffs); err != nil {
			return err
		}
	}
	return nil
}

func (m *Yale) deleteKey(keyId string, disabledAt time.Time, entry *cache.Entry, cutoffs cutoff.Cutoffs) error {
	// has enough time passed since this key was disabled? if not, do nothing
	logs.Info.Printf("key %s (service account %s) was disabled at %s, delete cutoff is %d days", keyId, entry.ServiceAccount.Email, disabledAt, cutoffs.DisableAfterDays())
	if !cutoffs.ShouldDelete(disabledAt) {
		logs.Info.Printf("key %s (service account %s): too early to delete", keyId, entry.ServiceAccount.Email, disabledAt, cutoffs.DisableAfterDays())
		return nil
	}

	logs.Info.Printf("key %s (service account %s) has reached delete cutoff; deleting it", keyId, entry.ServiceAccount.Email)
	if err := m.keyops.Delete(keyops.Key{
		Project:             entry.ServiceAccount.Project,
		ServiceAccountEmail: entry.ServiceAccount.Email,
		ID:                  keyId,
	}); err != nil {
		return fmt.Errorf("error deleting key %s (service account %s): %v", keyId, entry.ServiceAccount.Email, err)
	}

	delete(entry.DisabledKeys, keyId)
	if err := m.cache.Save(entry); err != nil {
		return fmt.Errorf("error updating cache entry for %s after key deletion: %v", entry.ServiceAccount.Email, err)
	}

	return nil
}
