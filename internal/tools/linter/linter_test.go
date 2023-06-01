package linter

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"path"
	"testing"
)

func Test_Linter(t *testing.T) {
	testCases := []struct {
		name     string
		expected []reference
	}{
		{
			name: "empty",
		},
		{
			name: "simple-missing",
			expected: []reference{
				{
					filename: "testdata/simple-missing/deployment.yaml",
					lineno:   12,
					kind:     "Deployment",
					name:     "deployment-1",
					secret:   "gsk-1-secret",
				},
			},
		},
		{
			name: "simple-auto-annotation",
		},
		{
			name: "simple-list-annotation",
		},
		{
			name: "simple-search-annotation",
		},
		{
			name: "simple-list-annotation-typo",
			expected: []reference{
				{
					filename: "testdata/simple-list-annotation-typo/deployment.yaml",
					lineno:   14,
					kind:     "Deployment",
					name:     "deployment-1",
					secret:   "gsk-1-secret",
				},
			},
		},
		{
			name: "sts-missing",
			expected: []reference{
				{
					filename: "testdata/sts-missing/sts.yaml",
					lineno:   12,
					kind:     "StatefulSet",
					name:     "sts-1",
					secret:   "gsk-1-secret",
				},
			},
		},
		{
			name: "complex-missing",
			expected: []reference{
				{
					filename: "testdata/complex-missing/deployment.yaml",
					lineno:   36,
					kind:     "Deployment",
					name:     "deployment-2",
					secret:   "gsk-2-secret",
				},
				{
					filename: "testdata/complex-missing/deployment.yaml",
					lineno:   52,
					kind:     "Deployment",
					name:     "deployment-3",
					secret:   "gsk-2-secret",
				},
				{
					filename: "testdata/complex-missing/deployment.yaml",
					lineno:   97,
					kind:     "Deployment",
					name:     "deployment-5",
					secret:   "gsk-1-secret",
				},
				{
					filename: "testdata/complex-missing/deployment.yaml",
					lineno:   103,
					kind:     "Deployment",
					name:     "deployment-5",
					secret:   "gsk-2-secret",
				},
				{
					filename: "testdata/complex-missing/sts.yaml",
					lineno:   12,
					kind:     "StatefulSet",
					name:     "sts-1",
					secret:   "gsk-1-secret",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := path.Join("testdata", tc.name)
			matches, err := Run(dir)
			if len(tc.expected) == 0 {
				require.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.ErrorContains(t, err, fmt.Sprintf("Found %d resources with missing annotations", len(tc.expected)))
			}
			assert.Equal(t, tc.expected, matches)
		})
	}
}
