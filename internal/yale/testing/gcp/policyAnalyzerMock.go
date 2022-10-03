package gcp

import (
	"context"
	"fmt"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
	"google.golang.org/api/policyanalyzer/v1"
	"net/http"
	"testing"
)

type PolicyAnalyzerMock interface {
	// Setup enables httpmock
	Setup()
	// GetClient returns a new iam.Service client that is configured to use httpmock
	GetClient() *policyanalyzer.Service
	// Verify verifies that all expectations on the mock client were met
	AssertExpectations(t *testing.T) bool
	// Cleanup disables httpmock
	Cleanup()
}

func NewPolicyAnaylzerMock(expectFn func(expect ExpectPolicyAnalyzer)) PolicyAnalyzerMock {
	e := newExpectPolicyAnalyzer()
	expectFn(e)

	httpClient := &http.Client{}
	policyAnaylzerClient, err := policyanalyzer.NewService(context.Background(), option.WithoutAuthentication(), option.WithHTTPClient(httpClient))
	if err != nil {
		panic(err)
	}

	return &policyAnaylzerMock{
		requests:             e.requests,
		httpClient:           httpClient,
		policyAnaylzerClient: policyAnaylzerClient,
	}
}

type policyAnaylzerMock struct {
	requests             []Request
	httpClient           *http.Client
	policyAnaylzerClient *policyanalyzer.Service
}

func (m *policyAnaylzerMock) Setup() {
	httpmock.ActivateNonDefault(m.httpClient)
	m.registerResponders()
}

func (m *policyAnaylzerMock) GetClient() *policyanalyzer.Service {
	return m.policyAnaylzerClient
}

func (m *policyAnaylzerMock) AssertExpectations(t *testing.T) bool {
	return assert.NoError(t, m.verifyCallCounts())
}

func (m *policyAnaylzerMock) Cleanup() {
	httpmock.DeactivateAndReset()
}

// registerResponders configures httpmock to respond to mocked requests
func (m *policyAnaylzerMock) registerResponders() {
	for _, r := range m.requests {
		httpmock.RegisterResponder(r.getMethod(), r.getUrl(), r.buildResponder())
	}
}

// verifyCallCounts verifies all mocked HTTP requests were made
func (m *policyAnaylzerMock) verifyCallCounts() error {
	counts := httpmock.GetCallCountInfo()
	for _, r := range m.requests {
		key := fmt.Sprintf("%s %s", r.getMethod(), r.getUrl())
		if counts[key] != r.getCallCount() {
			return fmt.Errorf("%s: %d calls expected, %d received", key, r.getCallCount(), counts[key])
		}
	}
	return nil
}
