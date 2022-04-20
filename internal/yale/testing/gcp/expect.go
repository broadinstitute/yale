package gcp

import (
	"fmt"
)

const gcpIamURL = "https://iam.googleapis.com/v1"

// Expect is an interface for setting expectations on a mock iam.Service
type Expect interface {
	// CreateServiceAccountKey configures the mock to expect a request to create a service account key
	CreateServiceAccountKey(project string, serviceAccountEmail string, hasError bool) CreateServiceAccountKeyRequest
	// GetServiceAccountKey configures the mock to expect a request to get a service account key
	GetServiceAccountKey(project string, serviceAccountEmail string, keyName string, hasError bool) GetServiceAccountKeyRequest
}

func newExpect() *expect {
	return &expect{}
}

// implements the Expect interface
type expect struct {
	requests []Request
}

// CreateServiceAccountKey
// see https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/create
func (e *expect) CreateServiceAccountKey(project string, serviceAccountEmail string, hasError bool) CreateServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/projects/%s/serviceAccounts/%s/keys", gcpIamURL, project, serviceAccountEmail)
	r := newCreateServiceAccountKeyRequest(methodPost, url)
	if hasError {
		r.Status(400)
	}
	e.addNewRequest(r)
	return r
}

// GetServiceAccountKey
// see https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/get
func (e *expect) GetServiceAccountKey(project string, serviceAccountEmail string, keyName string, hasError bool) GetServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/projects/%s/serviceAccounts/%s/keys/%s", gcpIamURL, project, serviceAccountEmail, keyName)
	r := newGetServiceAccountKeyRequest(methodGet, url)
	if hasError {
		r.Status(400)
	}
	return r
}

func (e *expect) addNewRequest(r Request) {
	e.requests = append(e.requests, r)
}
