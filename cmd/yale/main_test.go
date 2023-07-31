package main

import (
	"github.com/broadinstitute/yale/internal/yale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

const layout = "2006-01-02T15:04:05Z"

func Test_parseRotateWindow(t *testing.T) {
	now := parseTimeOrPanic("2023-07-31T09:11:22Z")

	testCases := []struct {
		name      string
		start     string
		end       string
		expectErr string
		expected  *yale.RotateWindow
	}{
		{
			name:     "neither flag set",
			expected: &yale.RotateWindow{},
		},
		{
			name:      "-window-start flag set",
			start:     "11:22",
			expectErr: "-window-start requires -window-end",
		},
		{
			name:      "-window-end flag set",
			end:       "11:22",
			expectErr: "-window-end requires -window-start",
		},
		{
			name:      "-window-end flag invalid",
			start:     "11:22",
			end:       "foo",
			expectErr: "-window-end: must be in HH:MM format",
		},
		{
			name:      "-window-start flag invalid",
			start:     "11 :22",
			end:       "13:01",
			expectErr: "-window-start: must be in HH:MM format",
		},
		{
			name:      "-window-start flag bad HH",
			start:     "99:22",
			end:       "13:01",
			expectErr: "-window-start: hour must be between 0 and 23",
		},
		{
			name:      "-window-start flag bad MM",
			start:     "07:22",
			end:       "13:99",
			expectErr: "-window-end: minute must be between 0 and 59",
		},
		{
			name:      "start after end",
			start:     "13:22",
			end:       "12:01",
			expectErr: "-window-start must be before -window-end",
		},
		{
			name:  "correct parsing",
			start: "07:34",
			end:   "15:01",
			expected: &yale.RotateWindow{
				Enabled:   true,
				StartTime: parseTimeOrPanic("2023-07-31T07:34:00Z"),
				EndTime:   parseTimeOrPanic("2023-07-31T15:01:00Z"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := &args{}

			if tc.start != "" {
				args.windowStart = tc.start
			}
			if tc.end != "" {
				args.windowEnd = tc.end
			}

			window, err := parseRotateWindow(args, now)
			if tc.expectErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.expectErr)
				return
			}

			assert.Equal(t, tc.expected, window)
		})
	}
}

func parseTimeOrPanic(value string) time.Time {
	t, err := time.Parse(layout, value)
	if err != nil {
		panic(err)
	}
	return t
}
