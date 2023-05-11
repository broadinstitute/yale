package cutoff

import (
	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"time"
)

type thresholds struct {
	rotateAfter  int
	disableAfter int
	deleteAfter  int
}

// minimums - the minimum supported value for a GSK's RotateAfter/DisableAfter/DeleteAfter
// attributes. If a user sets, for example, a RotateAfter value of 3, it will be rounded up to this minimum.
//
// Note that we should always choose minimum windows to account for delays in the API data that we use to
// determine if a key is still in use.
// With Cloud Monitoring Metrics, data can lag up to 6 hours behind realtime; 7 days is a very generous buffer.
var minimums = thresholds{
	rotateAfter:  7,
	disableAfter: 7,
	deleteAfter:  3,
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
	// RotateAfterDays Number of days to wait to rotate a key after issuing it (the basis for ShouldRotate)
	RotateAfterDays() int
	// DisableAfterDays Number of days to wait to disable a key before rotating it (the basis for ShouldDisable)
	DisableAfterDays() int
	// DeleteAfterDays Number of days to wait to delete a key before rotating it (the basis for ShouldDelete)
	DeleteAfterDays() int
}

func NewWithDefaults() Cutoffs {
	return newWithThresholds(minimums, time.Now())
}

func New(gsks ...apiv1b1.GCPSaKey) Cutoffs {
	return newWithCustomTime(gsks, time.Now())
}

func newWithCustomTime(gsks []apiv1b1.GCPSaKey, now time.Time) cutoffs {
	if len(gsks) < 1 {
		panic("at least one GcpSaKey must be supplied in order to compute cutoffs")
	}

	return newWithThresholds(computeThresholds(gsks), now)
}

func newWithThresholds(t thresholds, now time.Time) cutoffs {
	return cutoffs{
		now:        now,
		thresholds: t,
	}
}

type cutoffs struct {
	now        time.Time
	thresholds thresholds
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

func (c cutoffs) RotateAfterDays() int {
	return c.thresholds.rotateAfter
}

func (c cutoffs) DisableAfterDays() int {
	return c.thresholds.disableAfter
}

func (c cutoffs) DeleteAfterDays() int {
	return c.thresholds.deleteAfter
}

// rotateCutoff keys created before this timestamp should be rotated
func (c cutoffs) rotateCutoff() time.Time {
	return c.daysAgo(c.RotateAfterDays())
}

// disableCutoff keys rotated before this timestamp should be disabled (if they are unused)
func (c cutoffs) disableCutoff() time.Time {
	return c.daysAgo(c.DisableAfterDays())
}

// safeToDisableCutoff keys last authenticated before this timestamp should be safe to disable
func (c cutoffs) safeToDisableCutoff() time.Time {
	return c.now.Add(-1 * lastAuthSafeDisableBuffer)
}

// deleteCutoff keys disabled before this timestamp should be deleted
func (c cutoffs) deleteCutoff() time.Time {
	return c.daysAgo(c.DeleteAfterDays())
}

// computeCutoff compute a cutoff date N days in the past
func (c cutoffs) computeCutoff(ageDays int) time.Time {
	return c.daysAgo(ageDays)
}

// daysAgo return a timestamp that is n days in the past
func (c cutoffs) daysAgo(n int) time.Time {
	return c.now.Add(-1 * time.Duration(int64(n)*int64(oneDay)))
}

// computeThresholds take a set of gsks and collapse them into a set of agreed-upon thresholds
func computeThresholds(gsks []apiv1b1.GCPSaKey) thresholds {
	t := thresholds{
		rotateAfter: computeThreshold(gsks, func(gsk apiv1b1.GCPSaKey) int {
			return gsk.Spec.KeyRotation.RotateAfter
		}, minimums.rotateAfter, "RotateAfter"),
		disableAfter: computeThreshold(gsks, func(gsk apiv1b1.GCPSaKey) int {
			return gsk.Spec.KeyRotation.DisableAfter
		}, minimums.disableAfter, "DisableAfter"),
		deleteAfter: computeThreshold(gsks, func(gsk apiv1b1.GCPSaKey) int {
			return gsk.Spec.KeyRotation.DeleteAfter
		}, minimums.deleteAfter, "DeleteAfter"),
	}
	if len(gsks) > 1 {
		logs.Info.Printf("computed key rotation thresholds for %s from %d GSKs: rotate after %d days, disable after %d days, delete after %d days", gsks[0].Spec.GoogleServiceAccount.Name, len(gsks), t.rotateAfter, t.disableAfter, t.deleteAfter)
	}
	return t
}

// computeThreshold take the rotate/disable/delete days values from a list of GSKs and return the lowest value,
// rounding up to the hardcoded minimums/floors for each attribute if necessary
func computeThreshold(gsks []apiv1b1.GCPSaKey, fieldFn func(apiv1b1.GCPSaKey) int, floor int, fieldName string) int {
	min := gsks[0]
	for _, gsk := range gsks {
		v := fieldFn(gsk)
		minV := fieldFn(min)
		if v < minV {
			logs.Warn.Printf("found different %s values in GcpSaKey resources for %s: %s/%s=%d and %s/%s=%d", fieldName, gsk.Spec.GoogleServiceAccount.Name, min.Namespace, min.Name, minV, gsk.Namespace, gsk.Name, v)
			min = gsk
		}
	}

	minV := fieldFn(min)
	if minV < floor {
		logs.Warn.Printf("GcpSaKey %s/%s for %s has invalid %s value %d; rounding up to %d", min.Namespace, min.Name, min.Spec.GoogleServiceAccount.Name, fieldName, minV, floor)
		return floor
	}
	return minV
}
