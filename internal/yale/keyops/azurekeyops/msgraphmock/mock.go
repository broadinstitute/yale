package msgraphmock

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/manicminer/hamilton/msgraph"
	"github.com/stretchr/testify/assert"
)

type Mock interface {
	// Setup enables httpmock
	Setup()
	// AssertExpectations verifies that all expectations on the mock client were met
	AssertExpectations(t *testing.T) bool
	// Cleanup disables httpmock
	Cleanup()
}

func NewMockApplicationsClient(expectFn func(expect Expect)) *applicationsClientMock {
	e := newExpect()
	expectFn(e)

	applicationsClient := msgraph.NewApplicationsClient()
	httpClient := &http.Client{}
	applicationsClient.BaseClient.HttpClient = httpClient

	return &applicationsClientMock{
		requests:           e.requests,
		httpClient:         httpClient,
		applicationsClient: applicationsClient,
	}
}

type applicationsClientMock struct {
	requests           []Request
	httpClient         *http.Client
	applicationsClient *msgraph.ApplicationsClient
}

func (m *applicationsClientMock) Setup() {
	httpmock.ActivateNonDefault(m.httpClient)
	m.registerResponders()
}

func (m *applicationsClientMock) GetClient() *msgraph.ApplicationsClient {
	return m.applicationsClient
}

func (m *applicationsClientMock) AssertExpectations(t *testing.T) bool {
	return assert.NoError(t, m.verifyCallCounts())
}

func (m *applicationsClientMock) Cleanup() {
	httpmock.DeactivateAndReset()
}

func (m *applicationsClientMock) registerResponders() {
	for _, r := range m.requests {
		httpmock.RegisterResponder(r.getMethod(), r.getUrl(), r.buildResponder())
	}
}

func (m *applicationsClientMock) verifyCallCounts() error {
	counts := httpmock.GetCallCountInfo()
	for _, r := range m.requests {
		clientSecret := fmt.Sprintf("%s, %s", r.getMethod(), r.getUrl())
		if counts[clientSecret] != r.getCallCount() {
			return fmt.Errorf("%s: %d calls expected, %d received", clientSecret, r.getCallCount(), counts[clientSecret])
		}
	}
	return nil
}
