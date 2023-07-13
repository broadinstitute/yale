package cutoff

import (
	"fmt"
	"time"

	apiv1b1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/broadinstitute/yale/internal/yale/logs"
)

type thresholds struct {
	rotateAfter        int
	disableAfter       int
	deleteAfter        int
	ignoreUsageMetrics bool
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

type cutoffable interface {
	apiv1b1.GcpSaKey | apiv1b1.AzureClientSecret
}

func NewWithDefaults() Cutoffs {
	return newWithThresholds(minimums, time.Now())
}

func New[C cutoffable](cutoffables []C) Cutoffs {
	return newWithCustomTime(cutoffables, time.Now())
}

func newWithCustomTime[C cutoffable](cutoffables []C, now time.Time) cutoffs {
	if len(cutoffables) < 1 {
		panic("at least one GcpSaKey or AzureClientSecret must be supplied in order to compute cutoffs")
	}

	return newWithThresholds(computeThresholds(cutoffables), now)
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
	if c.thresholds.ignoreUsageMetrics {
		return true
	}
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

// daysAgo return a timestamp that is n days in the past
func (c cutoffs) daysAgo(n int) time.Time {
	return c.now.Add(-1 * time.Duration(int64(n)*int64(oneDay)))
}

// computeThresholds take a set of gsks and collapse them into a set of agreed-upon thresholds
func computeThresholds[C cutoffable](cutoffables []C) thresholds {
	switch cs := any(&cutoffables).(type) {
	case *[]apiv1b1.GcpSaKey:
		gsks := *cs
		t := thresholds{
			rotateAfter: computeThresholdGSK(gsks, func(gsk apiv1b1.GcpSaKey) int {
				return gsk.Spec.KeyRotation.RotateAfter
			}, minimums.rotateAfter, "RotateAfter"),
			disableAfter: computeThresholdGSK(gsks, func(gsk apiv1b1.GcpSaKey) int {
				return gsk.Spec.KeyRotation.DisableAfter
			}, minimums.disableAfter, "DisableAfter"),
			deleteAfter: computeThresholdGSK(gsks, func(gsk apiv1b1.GcpSaKey) int {
				return gsk.Spec.KeyRotation.DeleteAfter
			}, minimums.deleteAfter, "DeleteAfter"),
			ignoreUsageMetrics: computeIgnoreUsageMetricsGSK(gsks),
		}

		if len(cutoffables) > 1 {
			logs.Info.Printf("computed key rotation thresholds for %s from %d GSKs: rotate after %d days, disable after %d days, delete after %d days", gsks[0].Spec.GoogleServiceAccount.Name, len(gsks), t.rotateAfter, t.disableAfter, t.deleteAfter)
		}
		return t
	case *[]apiv1b1.AzureClientSecret:
		azureClientSecrets := *cs
		t := thresholds{
			rotateAfter: computeThresholdAzureClientSecret(azureClientSecrets, func(acs apiv1b1.AzureClientSecret) int {
				return acs.Spec.KeyRotation.RotateAfter
			}, minimums.rotateAfter, "RotateAfter"),
			disableAfter: computeThresholdAzureClientSecret(azureClientSecrets, func(acs apiv1b1.AzureClientSecret) int {
				return acs.Spec.KeyRotation.DisableAfter
			}, minimums.disableAfter, "DisableAfter"),
			deleteAfter: computeThresholdAzureClientSecret(azureClientSecrets, func(acs apiv1b1.AzureClientSecret) int {
				return acs.Spec.KeyRotation.DeleteAfter
			}, minimums.deleteAfter, "DeleteAfter"),
			ignoreUsageMetrics: computeIgnoreUsageMetricsAzureClientSecret(azureClientSecrets),
		}

		if len(cutoffables) > 1 {
			logs.Info.Printf("computed key rotation thresholds for %s from %d AzureClientSecrets: rotate after %d days, disable after %d days, delete after %d days", azureClientSecrets[0].Spec.AzureServicePrincipal.ApplicationID, len(azureClientSecrets), t.rotateAfter, t.disableAfter, t.deleteAfter)
		}
		return t

	default:
		panic(fmt.Sprintf("unknown yale resource type: %T", cutoffables))
	}
}

// computeThresholdGSK take the rotate/disable/delete days values from a list of GSKs and return the lowest value,
// rounding up to the hardcoded minimums/floors for each attribute if necessary
func computeThresholdGSK(gsks []apiv1b1.GcpSaKey, fieldFn func(apiv1b1.GcpSaKey) int, floor int, fieldName string) int {
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

func computeThresholdAzureClientSecret(azureClientSecrets []apiv1b1.AzureClientSecret, fieldFn func(apiv1b1.AzureClientSecret) int, floor int, fieldName string) int {
	min := azureClientSecrets[0]
	for _, azureClientSecret := range azureClientSecrets {
		v := fieldFn(azureClientSecret)
		minV := fieldFn(min)
		if v < minV {
			logs.Warn.Printf("found different %s values in AzureClientSecret resources for %s: %s/%s=%d and %s/%s=%d", fieldName, azureClientSecret.Spec.AzureServicePrincipal.ApplicationID, min.Namespace, min.Name, minV, azureClientSecret.Namespace, azureClientSecret.Name, v)
			min = azureClientSecret
		}
	}

	minV := fieldFn(min)
	if minV < floor {
		logs.Warn.Printf("AzureClientSecret %s/%s for %s has invalid %s value %d; rounding up to %d", min.Namespace, min.Name, min.Spec.AzureServicePrincipal.ApplicationID, fieldName, minV, floor)
		return floor
	}
	return minV
}

func computeIgnoreUsageMetricsGSK(gsks []apiv1b1.GcpSaKey) bool {
	if len(gsks) == 0 {
		return false
	}
	first := gsks[0]
	for _, gsk := range gsks {
		if gsk.Spec.KeyRotation.IgnoreUsageMetrics != first.Spec.KeyRotation.IgnoreUsageMetrics {
			logs.Warn.Printf("`IgnoreUsageMetrics` field differs between GcpSaKey resources for %s: %s/%s=%t and %s/%s=%t; usage metrics will not be ignored", gsk.Spec.GoogleServiceAccount.Name, first.Namespace, first.Name, first.Spec.KeyRotation.IgnoreUsageMetrics, gsk.Namespace, gsk.Name, gsk.Spec.KeyRotation.IgnoreUsageMetrics)
			return false
		}
	}
	return first.Spec.KeyRotation.IgnoreUsageMetrics
}

func computeIgnoreUsageMetricsAzureClientSecret(azureClientSecrets []apiv1b1.AzureClientSecret) bool {
	if len(azureClientSecrets) == 0 {
		return false
	}
	first := azureClientSecrets[0]
	for _, azureClientSecret := range azureClientSecrets {
		if azureClientSecret.Spec.KeyRotation.IgnoreUsageMetrics != first.Spec.KeyRotation.IgnoreUsageMetrics {
			logs.Warn.Printf("`IgnoreUsageMetrics` field differs between AzureClientSecret resources for %s: %s/%s=%t and %s/%s=%t; usage metrics will not be ignored", azureClientSecret.Spec.AzureServicePrincipal.ApplicationID, first.Namespace, first.Name, first.Spec.KeyRotation.IgnoreUsageMetrics, azureClientSecret.Namespace, azureClientSecret.Name, azureClientSecret.Spec.KeyRotation.IgnoreUsageMetrics)
			return false
		}
	}
	return first.Spec.KeyRotation.IgnoreUsageMetrics
}
