package resourcemap

import (
	"testing"

	"github.com/broadinstitute/yale/internal/yale/cache"
	cachemocks "github.com/broadinstitute/yale/internal/yale/cache/mocks"
	"github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
	crdmocks "github.com/broadinstitute/yale/internal/yale/crd/clientset/v1beta1/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var gsk1a = v1beta1.GcpSaKey{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gsk-1",
		Namespace: "ns-a",
	},
	Spec: v1beta1.GCPSaKeySpec{
		GoogleServiceAccount: v1beta1.GoogleServiceAccount{
			Name:    "sa-1@p.com",
			Project: "p",
		},
	},
}

var gsk1b = v1beta1.GcpSaKey{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gsk-1",
		Namespace: "ns-a",
	},
	Spec: v1beta1.GCPSaKeySpec{
		GoogleServiceAccount: v1beta1.GoogleServiceAccount{
			Name:    "sa-1@p.com",
			Project: "p",
		},
	},
}

var gsk2a = v1beta1.GcpSaKey{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gsk-2",
		Namespace: "ns-a",
	},
	Spec: v1beta1.GCPSaKeySpec{
		GoogleServiceAccount: v1beta1.GoogleServiceAccount{
			Name:    "sa-2@p.com",
			Project: "p",
		},
	},
}

var gsk2b = v1beta1.GcpSaKey{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gsk-2",
		Namespace: "ns-b",
	},
	Spec: v1beta1.GCPSaKeySpec{
		GoogleServiceAccount: v1beta1.GoogleServiceAccount{
			Name:    "sa-2@p.com",
			Project: "p",
		},
	},
}

var gsk2bBroken = v1beta1.GcpSaKey{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gsk-2",
		Namespace: "ns-b",
	},
	Spec: v1beta1.GCPSaKeySpec{
		GoogleServiceAccount: v1beta1.GoogleServiceAccount{
			Name:    "sa-2@p.com",
			Project: "mismatch", // wrong project - will mismatch cache entry / other gsks
		},
	},
}

var gsk4a = v1beta1.GcpSaKey{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gsk-4",
		Namespace: "ns-a",
	},
	Spec: v1beta1.GCPSaKeySpec{
		GoogleServiceAccount: v1beta1.GoogleServiceAccount{
			Name:    "sa-4@p.com",
			Project: "p",
		},
	},
}

var entry1 = &cache.Entry{
	ServiceAccount: cache.ServiceAccount{
		Email:   "sa-1@p.com",
		Project: "p",
	},
}

var entry2 = &cache.Entry{
	ServiceAccount: cache.ServiceAccount{
		Email:   "sa-2@p.com",
		Project: "p",
	},
}

var entry2Broken = &cache.Entry{
	ServiceAccount: cache.ServiceAccount{
		Email:   "sa-2@p.com",
		Project: "mismatch", // wrong project - will mismatch gsks
	},
}

var entry3 = &cache.Entry{
	ServiceAccount: cache.ServiceAccount{
		Email:   "sa-3@p.com",
		Project: "p",
	},
}

var entry4 = &cache.Entry{
	ServiceAccount: cache.ServiceAccount{
		Email:   "sa-4@p.com",
		Project: "p",
	},
}

func Test_Build(t *testing.T) {
	testCases := []struct {
		name                 string
		existingCacheEntries []*cache.Entry
		newCacheEntries      []*cache.Entry
		gsks                 []v1beta1.GcpSaKey
		// Azure Client Secrets
		acss      []v1beta1.AzureClientSecret
		expected  map[string]*Bundle
		expectErr string
	}{
		{
			name:     "empty cache, no gsks in cluster",
			expected: map[string]*Bundle{},
		},
		{
			name:            "empty cache, one gsk in cluster",
			gsks:            []v1beta1.GcpSaKey{gsk1a},
			acss:            []v1beta1.AzureClientSecret{},
			newCacheEntries: []*cache.Entry{entry1},
			expected: map[string]*Bundle{
				"sa-1@p.com": {
					Entry: entry1, // new entry created for sa-1
					GSKs:  []v1beta1.GcpSaKey{gsk1a},
				},
			},
		},
		{
			name:                 "one cache entry cache, matches one gsk in cluster",
			gsks:                 []v1beta1.GcpSaKey{gsk1a},
			acss:                 []v1beta1.AzureClientSecret{},
			existingCacheEntries: []*cache.Entry{entry1},
			expected: map[string]*Bundle{
				"sa-1@p.com": {
					Entry: entry1,
					GSKs:  []v1beta1.GcpSaKey{gsk1a},
				},
			},
		},
		{
			name:                 "one cache entry cache, matches two gsks in cluster",
			gsks:                 []v1beta1.GcpSaKey{gsk1a, gsk1b},
			acss:                 []v1beta1.AzureClientSecret{},
			existingCacheEntries: []*cache.Entry{entry1},
			expected: map[string]*Bundle{
				"sa-1@p.com": {
					Entry: entry1,
					GSKs:  []v1beta1.GcpSaKey{gsk1a, gsk1b},
				},
			},
		},
		{
			name:                 "broken cache entry should lead service account to be skipped",
			gsks:                 []v1beta1.GcpSaKey{gsk1a, gsk2a, gsk2b},
			acss:                 []v1beta1.AzureClientSecret{},
			existingCacheEntries: []*cache.Entry{entry1, entry2Broken},
			expected: map[string]*Bundle{
				"sa-1@p.com": {
					Entry: entry1,
					GSKs:  []v1beta1.GcpSaKey{gsk1a},
				},
			},
		},
		{
			name:                 "broken gsk should lead service account to be skipped",
			gsks:                 []v1beta1.GcpSaKey{gsk1a, gsk1b, gsk2a, gsk2bBroken},
			acss:                 []v1beta1.AzureClientSecret{},
			existingCacheEntries: []*cache.Entry{entry1, entry2},
			expected: map[string]*Bundle{
				"sa-1@p.com": {
					Entry: entry1,
					GSKs:  []v1beta1.GcpSaKey{gsk1a, gsk1b},
				},
			},
		},
		{
			name:                 "multiple entries and gsks",
			gsks:                 []v1beta1.GcpSaKey{gsk1a, gsk1b, gsk2a, gsk2b, gsk4a},
			acss:                 []v1beta1.AzureClientSecret{},
			existingCacheEntries: []*cache.Entry{entry1, entry2, entry3},
			newCacheEntries:      []*cache.Entry{entry4},
			expected: map[string]*Bundle{
				"sa-1@p.com": {
					Entry: entry1,
					GSKs:  []v1beta1.GcpSaKey{gsk1a, gsk1b},
				},
				"sa-2@p.com": {
					Entry: entry2,
					GSKs:  []v1beta1.GcpSaKey{gsk2a, gsk2b},
				},
				"sa-3@p.com": {
					Entry: entry3,
					GSKs:  nil,
				},
				"sa-4@p.com": {
					Entry: entry4, // new entry created for sa-4
					GSKs:  []v1beta1.GcpSaKey{gsk4a},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_cache := cachemocks.NewCache(t)
			_cache.EXPECT().List().Return(tc.existingCacheEntries, nil)

			for _, entry := range tc.newCacheEntries {
				_cache.EXPECT().GetOrCreate(cache.ServiceAccount{
					Email:   entry.ServiceAccount.Email,
					Project: entry.ServiceAccount.Project,
				}).Return(entry, nil)
			}

			gskEndpoint := crdmocks.NewGcpSaKeyInterface(t)
			crd := crdmocks.NewYaleCRDInterface(t)
			crd.EXPECT().GcpSaKeys().Return(gskEndpoint)

			acsEndpoint := crdmocks.NewAzureClientSecretInterface(t)
			crd.EXPECT().AzureClientSecrets().Return(acsEndpoint)

			gskEndpoint.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(&v1beta1.GCPSaKeyList{
				Items: tc.gsks,
			}, nil)

			acsEndpoint.EXPECT().List(mock.Anything, metav1.ListOptions{}).Return(&v1beta1.AzureClientSecretList{
				Items: tc.acss,
			}, nil)

			_mapper := New(crd, _cache)

			result, err := _mapper.Build()
			if tc.expectErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.expectErr)
				return
			}

			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_validateResourceBundle(t *testing.T) {
	testCases := []struct {
		name        string
		input       *Bundle
		errContains string
	}{
		{
			name: "should not return error if bundle has cache entry only",
			input: &Bundle{
				Entry: &cache.Entry{
					ServiceAccount: cache.ServiceAccount{
						Email:   "my-sa@p.com",
						Project: "p",
					},
				},
				GSKs: nil,
			},
			errContains: "",
		},
		{
			name: "should not error if bundle has gsk only",
			input: &Bundle{
				GSKs: []v1beta1.GcpSaKey{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gsk-1",
							Namespace: "ns-1",
						},
						Spec: v1beta1.GCPSaKeySpec{
							GoogleServiceAccount: v1beta1.GoogleServiceAccount{
								Project: "p",
							},
						},
					},
				},
			},
			errContains: "",
		},
		{
			name: "should not error if bundle and gsk match",
			input: &Bundle{
				Entry: &cache.Entry{
					ServiceAccount: cache.ServiceAccount{
						Email:   "my-sa@p.com",
						Project: "p",
					},
				},
				GSKs: []v1beta1.GcpSaKey{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gsk-1",
							Namespace: "ns-1",
						},
						Spec: v1beta1.GCPSaKeySpec{
							GoogleServiceAccount: v1beta1.GoogleServiceAccount{
								Project: "p",
							},
						},
					},
				},
			},
			errContains: "",
		},
		{
			name: "should not error if bundle and gsks all match",
			input: &Bundle{
				Entry: &cache.Entry{
					ServiceAccount: cache.ServiceAccount{
						Email:   "my-sa@p.com",
						Project: "p",
					},
				},
				GSKs: []v1beta1.GcpSaKey{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gsk-1",
							Namespace: "ns-1",
						},
						Spec: v1beta1.GCPSaKeySpec{
							GoogleServiceAccount: v1beta1.GoogleServiceAccount{
								Project: "p",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gsk-2",
							Namespace: "ns-2",
						},
						Spec: v1beta1.GCPSaKeySpec{
							GoogleServiceAccount: v1beta1.GoogleServiceAccount{
								Project: "p",
							},
						},
					},
				},
			},
			errContains: "",
		},
		{
			name: "should error if bundle and gsk do not match",
			input: &Bundle{
				Entry: &cache.Entry{
					ServiceAccount: cache.ServiceAccount{
						Email:   "my-sa@p.com",
						Project: "p",
					},
				},
				GSKs: []v1beta1.GcpSaKey{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gsk-1",
							Namespace: "ns-1",
						},
						Spec: v1beta1.GCPSaKeySpec{
							GoogleServiceAccount: v1beta1.GoogleServiceAccount{
								Project: "q",
							},
						},
					},
				},
			},
			errContains: "project mismatch",
		},
		{
			name: "should error if bundle and gsks do not all match",
			input: &Bundle{
				Entry: &cache.Entry{
					ServiceAccount: cache.ServiceAccount{
						Email:   "my-sa@p.com",
						Project: "p",
					},
				},
				GSKs: []v1beta1.GcpSaKey{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gsk-1",
							Namespace: "ns-1",
						},
						Spec: v1beta1.GCPSaKeySpec{
							GoogleServiceAccount: v1beta1.GoogleServiceAccount{
								Project: "p",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gsk-2",
							Namespace: "ns-2",
						},
						Spec: v1beta1.GCPSaKeySpec{
							GoogleServiceAccount: v1beta1.GoogleServiceAccount{
								Project: "q",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gsk-3",
							Namespace: "ns-3",
						},
						Spec: v1beta1.GCPSaKeySpec{
							GoogleServiceAccount: v1beta1.GoogleServiceAccount{
								Project: "p",
							},
						},
					},
				},
			},
			errContains: "project mismatch",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateResourceBundle(tc.input)
			if tc.errContains == "" {
				require.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.errContains)
			}
		})
	}
}
