package yale

import (
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/cache"
	"github.com/broadinstitute/yale/internal/yale/cutoff"
	"github.com/broadinstitute/yale/internal/yale/keyops"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"time"
)

func (m *Yale) disableOldKeys(entry *cache.Entry, cutoffs cutoff.Cutoffs) error {
	for keyId, rotatedAt := range entry.DisabledKeys {
		if err := m.disableOneKey(keyId, rotatedAt, entry, cutoffs); err != nil {
			return err
		}
	}
	return nil
}

func (m *Yale) disableOneKey(keyId string, rotatedAt time.Time, entry *cache.Entry, cutoffs cutoff.Cutoffs) error {
	// has enough time passed since rotation? if not, do nothing
	logs.Info.Printf("key %s (service account %s) was rotated at %s, disable cutoff is %d days", keyId, entry.ServiceAccount.Email, rotatedAt, cutoffs.DisableAfterDays())
	if !cutoffs.ShouldDisable(rotatedAt) {
		logs.Info.Printf("key %s (service account %s): too early to disable", keyId, entry.ServiceAccount.Email, rotatedAt, cutoffs.DisableAfterDays())
		return nil
	}

	// check if the key is still in use
	logs.Info.Printf("key %s (service account %s) has reached disable cutoff; checking if still in use", keyId, entry.ServiceAccount.Email)
	lastAuthTime, err := m.authmetrics.LastAuthTime(entry.ServiceAccount.Project, entry.ServiceAccount.Email, keyId)
	if err != nil {
		return fmt.Errorf("error determining last authentication time for key %s (service account %s): %v", keyId, entry.ServiceAccount.Email, err)
	}
	if lastAuthTime == nil {
		logs.Info.Printf("could not identify last authentication time for key %s (service account %s); assuming key is not in use", keyId, entry.ServiceAccount.Email)
	} else {
		logs.Info.Printf("last authentication time for key %s (service account %s): %s", keyId, entry.ServiceAccount.Email, *lastAuthTime)
		if !cutoffs.SafeToDisable(*lastAuthTime) {
			return fmt.Errorf("key %s (service account %s) was rotated at %s but was last used to authenticate at %s; please find out what's still using this key and fix it", keyId, entry.ServiceAccount.Email, rotatedAt, *lastAuthTime)
		}
	}

	// disable the key
	logs.Info.Printf("disabling key %s (service account %s)...", keyId, entry.ServiceAccount.Email)
	if err = m.keyops.EnsureDisabled(keyops.Key{
		Project:             entry.ServiceAccount.Project,
		ServiceAccountEmail: entry.ServiceAccount.Email,
		ID:                  keyId,
	}); err != nil {
		return fmt.Errorf("error disabling key %s (service account %s): %v", keyId, entry.ServiceAccount.Email, err)
	}

	// update cache entry to reflect that the key was successfully disabled
	delete(entry.RotatedKeys, keyId)
	entry.DisabledKeys[keyId] = time.Now()
	if err = m.cache.Save(entry); err != nil {
		return fmt.Errorf("error saving cache entry after key disable: %v", err)
	}
	return nil
}
