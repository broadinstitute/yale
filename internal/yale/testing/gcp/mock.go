package gcp

import (
	"context"
	"fmt"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/policyanalyzer/v1"
	"net/http"
	"testing"
)

type Mock interface {
	// Setup enables httpmock
	Setup()
	// GetIAMClient returns a new iam.Service client that is configured to use httpmock
	GetIAMClient() *iam.Service
	GetPAClient() *policyanalyzer.Service
	// AssertExpectations verifies that all expectations on the mock client were met
	AssertExpectations(t *testing.T) bool
	// Cleanup disables httpmock
	Cleanup()
}

func NewMock(expectFn func(expect Expect)) Mock {
	e := newExpect()
	expectFn(e)

	httpClient := &http.Client{}
	iamClient, err := iam.NewService(context.Background(), option.WithoutAuthentication(), option.WithHTTPClient(httpClient))
	if err != nil {
		panic(err)
	}
	paClient, err := policyanalyzer.NewService(context.Background(), option.WithoutAuthentication(), option.WithHTTPClient(httpClient))
	if err != nil {
		panic(err)
	}

	return &mock{
		requests:   e.requests,
		httpClient: httpClient,
		iamClient:  iamClient,
		paClient: paClient,
	}
}

type mock struct {
	requests   []Request
	httpClient *http.Client
	iamClient  *iam.Service
	paClient *policyanalyzer.Service
}

func (m *mock) Setup() {
	httpmock.ActivateNonDefault(m.httpClient)
	m.registerResponders()
}

func (m *mock) GetIAMClient() *iam.Service {
	return m.iamClient
}

func (m *mock) GetPAClient() *policyanalyzer.Service {
	return m.paClient
}

func (m *mock) AssertExpectations(t *testing.T) bool {
	return assert.NoError(t, m.verifyCallCounts())
}

func (m *mock) Cleanup() {
	httpmock.DeactivateAndReset()
}

// registerResponders configures httpmock to respond to mocked requests
func (m *mock) registerResponders() {
	for _, r := range m.requests {
		httpmock.RegisterResponder(r.getMethod(), r.getUrl(), r.buildResponder())
	}
}

// verifyCallCounts verifies all mocked HTTP requests were made
func (m *mock) verifyCallCounts() error {
	counts := httpmock.GetCallCountInfo()
	for _, r := range m.requests {
		key := fmt.Sprintf("%s %s", r.getMethod(), r.getUrl())
		if counts[key] != r.getCallCount() {
			return fmt.Errorf("%s: %d calls expected, %d received", key, r.getCallCount(), counts[key])
		}
	}
	return nil
}