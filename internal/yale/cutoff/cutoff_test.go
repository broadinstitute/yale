package cutoff

import (
	"testing"
	"time"

	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_Cutoffs(t *testing.T) {
	layout := time.RFC3339
	now, err := time.Parse(layout, "2023-04-28T09:10:11Z")
	require.NoError(t, err)

	type cutoffTimes struct {
		rotateCutoff        string
		disableCutoff       string
		safeToDisableCutoff string
		deleteCutoff        string
	}

	type shouldChecks struct {
		input       string
		rotate      bool
		disable     bool
		safeDisable bool
		delete      bool
	}

	testCases := []struct {
		name               string
		input              v1beta1.KeyRotation
		expectedThresholds thresholds
		expectedCutoffs    cutoffTimes
		shouldChecks       []shouldChecks
	}{
		{
			name: "should round up to configured minimums",
			input: v1beta1.KeyRotation{
				RotateAfter:  -1,
				DisableAfter: 0,
				DeleteAfter:  1,
			},
			expectedThresholds: thresholds{
				rotateAfter:  7,
				disableAfter: 7,
				deleteAfter:  3,
			},
			expectedCutoffs: cutoffTimes{
				rotateCutoff:        "2023-04-21T09:10:11Z",
				disableCutoff:       "2023-04-21T09:10:11Z",
				safeToDisableCutoff: "2023-04-25T09:10:11Z",
				deleteCutoff:        "2023-04-25T09:10:11Z",
			},
			shouldChecks: []shouldChecks{
				{
					input:       "2023-04-14T09:05:11Z",
					rotate:      true,
					disable:     true,
					safeDisable: true,
					delete:      true,
				},
				{
					input:       "2023-04-21T09:15:11Z",
					rotate:      false,
					disable:     false,
					safeDisable: true,
					delete:      true,
				},
				{
					input:       "2023-04-25T09:09:11Z",
					rotate:      false,
					disable:     false,
					safeDisable: true,
					delete:      true,
				},
				{
					input:       "2023-04-25T09:11:11Z",
					rotate:      false,
					disable:     false,
					safeDisable: false,
					delete:      false,
				},
			},
		},
		{
			name: "should return correct cutoffs for values above minimums",
			input: v1beta1.KeyRotation{
				RotateAfter:  17,
				DisableAfter: 16,
				DeleteAfter:  8,
			},
			expectedThresholds: thresholds{
				rotateAfter:  17,
				disableAfter: 16,
				deleteAfter:  8,
			},
			expectedCutoffs: cutoffTimes{
				rotateCutoff:        "2023-04-11T09:10:11Z",
				disableCutoff:       "2023-04-12T09:10:11Z",
				safeToDisableCutoff: "2023-04-25T09:10:11Z",
				deleteCutoff:        "2023-04-20T09:10:11Z",
			},
			shouldChecks: []shouldChecks{
				{
					input:       "2023-04-27T00:00:00Z",
					rotate:      false,
					disable:     false,
					safeDisable: false,
					delete:      false,
				},
				{
					input:       "2023-04-23T00:00:00Z",
					rotate:      false,
					disable:     false,
					safeDisable: true,
					delete:      false,
				},
				{
					input:       "2023-04-15T09:00:00Z",
					rotate:      false,
					disable:     false,
					safeDisable: true,
					delete:      true,
				},
				{
					input:       "2023-04-11T09:30:00Z",
					rotate:      false,
					disable:     true,
					safeDisable: true,
					delete:      true,
				},
				{
					input:       "2023-04-11T08:59:00Z",
					rotate:      true,
					disable:     true,
					safeDisable: true,
					delete:      true,
				},
			},
		},
		{
			name: "should always return safe to disable if ignore usage metrics is true",
			input: v1beta1.KeyRotation{
				RotateAfter:        7,
				DisableAfter:       7,
				DeleteAfter:        3,
				IgnoreUsageMetrics: true,
			},
			expectedThresholds: thresholds{
				rotateAfter:  7,
				disableAfter: 7,
				deleteAfter:  3,
			},
			expectedCutoffs: cutoffTimes{
				rotateCutoff:        "2023-04-21T09:10:11Z",
				disableCutoff:       "2023-04-21T09:10:11Z",
				safeToDisableCutoff: "2023-04-25T09:10:11Z",
				deleteCutoff:        "2023-04-25T09:10:11Z",
			},
			shouldChecks: []shouldChecks{
				{
					input:       "2023-04-27T00:00:00Z",
					rotate:      false,
					disable:     false,
					safeDisable: true,
					delete:      false,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gsk := v1beta1.GcpSaKey{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gsk",
					Namespace: "test-namespace",
				},
				Spec: v1beta1.GCPSaKeySpec{
					KeyRotation: tc.input,
				},
			}
			c := newWithCustomTime([]v1beta1.GcpSaKey{gsk}, now)

			assert.Equal(t, tc.expectedThresholds.rotateAfter, c.RotateAfterDays())
			assert.Equal(t, tc.expectedThresholds.disableAfter, c.DisableAfterDays())
			assert.Equal(t, tc.expectedThresholds.deleteAfter, c.DeleteAfterDays())

			assert.Equal(t, tc.expectedCutoffs.rotateCutoff, c.rotateCutoff().Format(layout))
			assert.Equal(t, tc.expectedCutoffs.disableCutoff, c.disableCutoff().Format(layout))
			assert.Equal(t, tc.expectedCutoffs.safeToDisableCutoff, c.safeToDisableCutoff().Format(layout))
			assert.Equal(t, tc.expectedCutoffs.deleteCutoff, c.deleteCutoff().Format(layout))

			for _, sc := range tc.shouldChecks {
				timestamp, err := time.Parse(layout, sc.input)
				require.NoError(t, err, "input: %q", sc.input)

				assert.Equal(t, sc.rotate, c.ShouldRotate(timestamp), "input: %q", sc.input)
				assert.Equal(t, sc.disable, c.ShouldDisable(timestamp), "input: %q", sc.input)
				assert.Equal(t, sc.safeDisable, c.SafeToDisable(timestamp), "input: %q", sc.input)
				assert.Equal(t, sc.delete, c.ShouldDelete(timestamp), "input: %q", sc.input)
			}
		})
	}

	// test with v1beta1.AzureClientSecret too
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			azureClientSecret := v1beta1.AzureClientSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-azureClientSecret",
					Namespace: "test-namespace",
				},
				Spec: v1beta1.AzureClientSecretSpec{
					KeyRotation: tc.input,
				},
			}
			c := newWithCustomTime([]v1beta1.AzureClientSecret{azureClientSecret}, now)

			assert.Equal(t, tc.expectedThresholds.rotateAfter, c.RotateAfterDays())
			assert.Equal(t, tc.expectedThresholds.disableAfter, c.DisableAfterDays())
			assert.Equal(t, tc.expectedThresholds.deleteAfter, c.DeleteAfterDays())

			assert.Equal(t, tc.expectedCutoffs.rotateCutoff, c.rotateCutoff().Format(layout))
			assert.Equal(t, tc.expectedCutoffs.disableCutoff, c.disableCutoff().Format(layout))
			assert.Equal(t, tc.expectedCutoffs.safeToDisableCutoff, c.safeToDisableCutoff().Format(layout))
			assert.Equal(t, tc.expectedCutoffs.deleteCutoff, c.deleteCutoff().Format(layout))

			for _, sc := range tc.shouldChecks {
				timestamp, err := time.Parse(layout, sc.input)
				require.NoError(t, err, "input: %q", sc.input)

				assert.Equal(t, sc.rotate, c.ShouldRotate(timestamp), "input: %q", sc.input)
				assert.Equal(t, sc.disable, c.ShouldDisable(timestamp), "input: %q", sc.input)
				assert.Equal(t, sc.safeDisable, c.SafeToDisable(timestamp), "input: %q", sc.input)
				assert.Equal(t, sc.delete, c.ShouldDelete(timestamp), "input: %q", sc.input)
			}
		})
	}
}

func Test_computeThresholds(t *testing.T) {
	testCases := []struct {
		name     string
		input    []v1beta1.GcpSaKey
		expected thresholds
	}{
		{
			name: "should return correct thresholds for a single gsk",
			input: []v1beta1.GcpSaKey{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-1",
						Namespace: "test-namespace",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  7,
							DisableAfter: 8,
							DeleteAfter:  9,
						},
						GoogleServiceAccount: v1beta1.GoogleServiceAccount{
							Name: "my-sa@p.com",
						},
					},
				},
			},
			expected: thresholds{
				rotateAfter:  7,
				disableAfter: 8,
				deleteAfter:  9,
			},
		},
		{
			name: "should round up to configured minimums",
			input: []v1beta1.GcpSaKey{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-1",
						Namespace: "test-namespace",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  -1,
							DisableAfter: 0,
							DeleteAfter:  1,
						},
						GoogleServiceAccount: v1beta1.GoogleServiceAccount{
							Name: "my-sa@p.com",
						},
					},
				},
			},
			expected: thresholds{
				rotateAfter:  7,
				disableAfter: 7,
				deleteAfter:  3,
			},
		},
		{
			name: "should choose minimum valid value for multiple conflicting GSK specs",
			input: []v1beta1.GcpSaKey{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-1",
						Namespace: "test-ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  7,
							DisableAfter: 12,
							DeleteAfter:  1,
						},
						GoogleServiceAccount: v1beta1.GoogleServiceAccount{
							Name: "my-sa@p.com",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-2",
						Namespace: "test-ns-2",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  6,
							DisableAfter: 9,
							DeleteAfter:  2,
						},
						GoogleServiceAccount: v1beta1.GoogleServiceAccount{
							Name: "my-sa@p.com",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-3",
						Namespace: "test-ns-3",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  8,
							DisableAfter: 22,
							DeleteAfter:  1,
						},
						GoogleServiceAccount: v1beta1.GoogleServiceAccount{
							Name: "my-sa@p.com",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-4",
						Namespace: "test-ns-4",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  2,
							DisableAfter: 17,
							DeleteAfter:  0,
						},
						GoogleServiceAccount: v1beta1.GoogleServiceAccount{
							Name: "my-sa@p.com",
						},
					},
				},
			},
			expected: thresholds{
				rotateAfter:  7,
				disableAfter: 9,
				deleteAfter:  3,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, computeThresholds(tc.input))
		})
	}
}

func Test_computeThresholdsAzureClientSecrets(t *testing.T) {
	testCases := []struct {
		name     string
		input    []v1beta1.AzureClientSecret
		expected thresholds
	}{
		{
			name: "should return correct thresholds for a single gsk",
			input: []v1beta1.AzureClientSecret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-1",
						Namespace: "test-namespace",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  7,
							DisableAfter: 8,
							DeleteAfter:  9,
						},
						AzureServicePrincipal: v1beta1.AzureServicePrincipal{
							ApplicationID: "test-application-id",
							TenantID:      "test-tenant-id",
						},
					},
				},
			},
			expected: thresholds{
				rotateAfter:  7,
				disableAfter: 8,
				deleteAfter:  9,
			},
		},
		{
			name: "should round up to configured minimums",
			input: []v1beta1.AzureClientSecret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-1",
						Namespace: "test-namespace",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  -1,
							DisableAfter: 0,
							DeleteAfter:  1,
						},
						AzureServicePrincipal: v1beta1.AzureServicePrincipal{
							ApplicationID: "test-application-id",
							TenantID:      "test-tenant-id",
						},
					},
				},
			},
			expected: thresholds{
				rotateAfter:  7,
				disableAfter: 7,
				deleteAfter:  3,
			},
		},
		{
			name: "should choose minimum valid value for multiple conflicting GSK specs",
			input: []v1beta1.AzureClientSecret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-1",
						Namespace: "test-ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  7,
							DisableAfter: 12,
							DeleteAfter:  1,
						},
						AzureServicePrincipal: v1beta1.AzureServicePrincipal{
							ApplicationID: "test-application-id",
							TenantID:      "test-tenant-id",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-2",
						Namespace: "test-ns-2",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  6,
							DisableAfter: 9,
							DeleteAfter:  2,
						},
						AzureServicePrincipal: v1beta1.AzureServicePrincipal{
							ApplicationID: "test-application-id",
							TenantID:      "test-tenant-id",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-3",
						Namespace: "test-ns-3",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  8,
							DisableAfter: 22,
							DeleteAfter:  1,
						},
						AzureServicePrincipal: v1beta1.AzureServicePrincipal{
							ApplicationID: "test-application-id",
							TenantID:      "test-tenant-id",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gsk-4",
						Namespace: "test-ns-4",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							RotateAfter:  2,
							DisableAfter: 17,
							DeleteAfter:  0,
						},
						AzureServicePrincipal: v1beta1.AzureServicePrincipal{
							ApplicationID: "test-application-id",
							TenantID:      "test-tenant-id",
						},
					},
				},
			},
			expected: thresholds{
				rotateAfter:  7,
				disableAfter: 9,
				deleteAfter:  3,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, computeThresholds(tc.input))
		})
	}
}

func Test_computeIgnoreUsageMetrics(t *testing.T) {
	testCases := []struct {
		name     string
		input    []v1beta1.GcpSaKey
		expected bool
	}{
		{
			name:     "empty",
			input:    []v1beta1.GcpSaKey{},
			expected: false,
		},
		{
			name: "single gsk with ignoreUsageMetrics set to false",
			input: []v1beta1.GcpSaKey{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-1",
						Namespace: "ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: false,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "single gsk with ignoreUsageMetrics set to true",
			input: []v1beta1.GcpSaKey{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-1",
						Namespace: "ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "multiple gsks with ignoreUsageMetrics set to true",
			input: []v1beta1.GcpSaKey{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-1",
						Namespace: "ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-2",
						Namespace: "ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-3",
						Namespace: "ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "multiple gsks with ignoreUsageMetrics set to false",
			input: []v1beta1.GcpSaKey{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-1",
						Namespace: "ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: false,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-2",
						Namespace: "ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: false,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-3",
						Namespace: "ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: false,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "multiple gsks with ignoreUsageMetrics set to true and false",
			input: []v1beta1.GcpSaKey{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-1",
						Namespace: "ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-2",
						Namespace: "ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: false,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-3",
						Namespace: "ns-1",
					},
					Spec: v1beta1.GCPSaKeySpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, computeIgnoreUsageMetricsGSK(tc.input))
		})
	}
}

func Test_computeIgnoreUsageMetricsAzureClientSecrets(t *testing.T) {
	testCases := []struct {
		name     string
		input    []v1beta1.AzureClientSecret
		expected bool
	}{
		{
			name:     "empty",
			input:    []v1beta1.AzureClientSecret{},
			expected: false,
		},
		{
			name: "single gsk with ignoreUsageMetrics set to false",
			input: []v1beta1.AzureClientSecret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-1",
						Namespace: "ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: false,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "single gsk with ignoreUsageMetrics set to true",
			input: []v1beta1.AzureClientSecret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-1",
						Namespace: "ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "multiple gsks with ignoreUsageMetrics set to true",
			input: []v1beta1.AzureClientSecret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-1",
						Namespace: "ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-2",
						Namespace: "ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-3",
						Namespace: "ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "multiple gsks with ignoreUsageMetrics set to false",
			input: []v1beta1.AzureClientSecret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-1",
						Namespace: "ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: false,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-2",
						Namespace: "ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: false,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-3",
						Namespace: "ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: false,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "multiple gsks with ignoreUsageMetrics set to true and false",
			input: []v1beta1.AzureClientSecret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-1",
						Namespace: "ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-2",
						Namespace: "ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: false,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gsk-3",
						Namespace: "ns-1",
					},
					Spec: v1beta1.AzureClientSecretSpec{
						KeyRotation: v1beta1.KeyRotation{
							IgnoreUsageMetrics: true,
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, computeIgnoreUsageMetricsAzureClientSecret(tc.input))
		})
	}
}
