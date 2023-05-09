package gcp

import (
	"context"
	"fmt"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	"net/http"
	"testing"
)

type Mock interface {
	// Setup enables httpmock
	Setup()
	// Verify verifies that all expectations on the mock client were met
	AssertExpectations(t *testing.T) bool
	// Cleanup disables httpmock
	Cleanup()
}

func NewMockIAMService(expectFn func(expect Expect)) *iamMock {
	e := newExpect()
	expectFn(e)

	httpClient := &http.Client{}
	iamClient, err := iam.NewService(context.Background(), option.WithoutAuthentication(), option.WithHTTPClient(httpClient))
	if err != nil {
		panic(err)
	}

	return &iamMock{
		requests:   e.requests,
		httpClient: httpClient,
		iamClient:  iamClient,
	}
}

type iamMock struct {
	requests   []Request
	httpClient *http.Client
	iamClient  *iam.Service
}

func (m *iamMock) Setup() {
	httpmock.ActivateNonDefault(m.httpClient)
	m.registerResponders()
}

func (m *iamMock) GetClient() *iam.Service {
	return m.iamClient
}

func (m *iamMock) AssertExpectations(t *testing.T) bool {
	return assert.NoError(t, m.verifyCallCounts())
}

func (m *iamMock) Cleanup() {
	httpmock.DeactivateAndReset()
}

// registerResponders configures httpmock to respond to mocked requests
func (m *iamMock) registerResponders() {
	for _, r := range m.requests {
		httpmock.RegisterResponder(r.getMethod(), r.getUrl(), r.buildResponder())
	}
}

// verifyCallCounts verifies all mocked HTTP requests were made
func (m *iamMock) verifyCallCounts() error {
	counts := httpmock.GetCallCountInfo()
	for _, r := range m.requests {
		key := fmt.Sprintf("%s %s", r.getMethod(), r.getUrl())
		if counts[key] != r.getCallCount() {
			return fmt.Errorf("%s: %d calls expected, %d received", key, r.getCallCount(), counts[key])
		}
	}
	return nil
}
