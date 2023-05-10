package cutoff

import (
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"time"
)

// minimums - the minimum supported value for a GSK's RotateAfter/DisableAfter/DeleteAfter
// attributes. If a user sets, for example, a RotateAfter value of 3, it will be rounded up to this minimum.
//
// Note that we should always choose minimum windows to account for delays in the API data that we use to
// determine if a key is still in use.
// With Cloud Monitoring Metrics, data can lag up to 6 hours behind realtime; 7 days is a very generous buffer.
var minimums = struct {
	RotateAfter  int
	DisableAfter int
	DeleteAfter  int
}{
	RotateAfter:  7,
	DisableAfter: 7,
	DeleteAfter:  3,
}

// oneDay time.Duration representing time in a single day
const oneDay = 24 * time.Hour

// lastAuthSafeDisableBuffer consider a key safe to disable if it has not been used within this much time
const lastAuthSafeDisableBuffer = 3 * oneDay

// Cutoffs is responsible for determining when a service account key should be rotated, disabled, or deleted
type Cutoffs interface {
	// ShouldRotate Return true if the key created at the given timestamp should be rotated
	ShouldRotate(createdAt time.Time) bool
	// ShouldDisable Return true if the key rotated at the given timestamp should be disabled
	ShouldDisable(rotatedAt time.Time) bool
	// SafeToDisable Return true if the key rotated at the given timestamp is safe to disable
	SafeToDisable(lastAuthTime time.Time) bool
	// ShouldDelete Return true if the key disabled at the given timestamp should be deleted
	ShouldDelete(disabledAt time.Time) bool
	// DisableAfterDays Number of days to wait to disable a key before rotating it (the basis for ShouldDisable)
	DisableAfterDays() int
	// DeleteAfterDays Number of days to wait to delete a key before rotating it (the basis for ShouldDelete)
	DeleteAfterDays() int
}

func New(gsk apiv1b1.GCPSaKey) Cutoffs {
	return cutoffs{
		gsk: gsk,
		now: time.Now(),
	}
}

type cutoffs struct {
	gsk apiv1b1.GCPSaKey
	now time.Time
}

// ShouldRotate Return true if the key created at the given timestamp should be rotated
func (c cutoffs) ShouldRotate(createdAt time.Time) bool {
	return createdAt.Before(c.rotateCutoff())
}

func (c cutoffs) ShouldDisable(rotatedAt time.Time) bool {
	return rotatedAt.Before(c.disableCutoff())
}

func (c cutoffs) SafeToDisable(lastAuthTime time.Time) bool {
	return lastAuthTime.Before(c.safeToDisableCutoff())
}

func (c cutoffs) ShouldDelete(disabledAt time.Time) bool {
	return disabledAt.Before(c.deleteCutoff())
}

func (c cutoffs) DisableAfterDays() int {
	return c.gsk.Spec.KeyRotation.DisableAfter
}

func (c cutoffs) DeleteAfterDays() int {
	return c.gsk.Spec.KeyRotation.DeleteAfter
}

// rotateCutoff keys created before this timestamp should be rotated
func (c cutoffs) rotateCutoff() time.Time {
	return c.computeCutoff(c.gsk.Spec.KeyRotation.RotateAfter, minimums.RotateAfter, "RotateAfter")
}

// disableCutoff keys rotated before this timestamp should be disabled (if they are unused)
func (c cutoffs) disableCutoff() time.Time {
	return c.computeCutoff(c.DisableAfterDays(), minimums.DisableAfter, "DisableAfter")
}

// safeToDisableCutoff keys last authenticated before this timestamp should be safe to disable
func (c cutoffs) safeToDisableCutoff() time.Time {
	return c.now.Add(-1 * lastAuthSafeDisableBuffer)
}

// deleteCutoff keys disabled before this timestamp should be deleted
func (c cutoffs) deleteCutoff() time.Time {
	return c.computeCutoff(c.gsk.Spec.KeyRotation.DeleteAfter, minimums.DeleteAfter, "DeleteAfter")
}

// computeCutoff compute a cutoff date N days in the past
func (c cutoffs) computeCutoff(ageDays int, minDays int, attrName string) time.Time {
	ageDays = c.ensureMininum(ageDays, minDays, attrName)
	return c.daysAgo(ageDays)
}

// ensureMinimum given a number of days, if it's less than minOperationWindowDays, round it up to the min and log a warning
func (c cutoffs) ensureMininum(ageDays int, minDays int, attrName string) int {
	if ageDays < minDays {
		logs.Warn.Printf("%s in %s: %s has invalid value %d, will round up to %d", c.gsk.Name, c.gsk.Namespace, attrName, ageDays, minDays)
		return minDays
	}
	return ageDays
}

// daysAgo return a timestamp that is n days in the past
func (c cutoffs) daysAgo(n int) time.Time {
	return c.now.Add(-1 * time.Duration(int64(n)*int64(oneDay)))
}
