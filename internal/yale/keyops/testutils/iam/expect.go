package gcp

import (
	"fmt"
	"net/http"
)

const gcpIamURL = "https://iam.googleapis.com/v1"

// Expect is an interface for setting expectations on a mock iam.Service
type Expect interface {
	// CreateServiceAccountKey configures the mock to expect a request to create a service account key
	CreateServiceAccountKey(project string, serviceAccountEmail string) CreateServiceAccountKeyRequest
	// GetServiceAccountKey configures the mock to expect a request to get a service account key
	GetServiceAccountKey(project string, serviceAccountEmail string, keyId string) GetServiceAccountKeyRequest
	// DisableServiceAccountKey configures the mock to expect a request to disable a service account key
	DisableServiceAccountKey(project string, serviceAccountEmail string, keyId string) DisableServiceAccountKeyRequest
	// DeleteServiceAccountKey configures the mock to expect a request that deletes a service account key
	DeleteServiceAccountKey(project string, serviceAccountEmail string, keyId string) DeleteServiceAccountKeyRequest
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
func (e *expect) CreateServiceAccountKey(project string, serviceAccountEmail string) CreateServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/projects/%s/serviceAccounts/%s/keys", gcpIamURL, project, serviceAccountEmail)
	r := newCreateServiceAccountKeyRequest(methodPost, url)
	e.addNewRequest(r)
	return r
}

// GetServiceAccountKey
// see https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/get
func (e *expect) GetServiceAccountKey(project string, serviceAccountEmail string, keyId string) GetServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/projects/%s/serviceAccounts/%s/keys/%s", gcpIamURL, project, serviceAccountEmail, keyId)
	r := newGetServiceAccountKeyRequest(methodGet, url)
	e.addNewRequest(r)
	return r
}

// DisableServiceAccountKey
// see https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/disable
func (e *expect) DisableServiceAccountKey(project string, serviceAccountEmail string, keyId string) DisableServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/projects/%s/serviceAccounts/%s/keys/%s:disable", gcpIamURL, project, serviceAccountEmail, keyId)
	r := newDisableServiceAccountKeyRequest(methodPost, url)
	e.addNewRequest(r)
	return r
}

// DeleteServiceAccountKey
// see https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/delete
func (e *expect) DeleteServiceAccountKey(project string, serviceAccountEmail string, keyId string) DeleteServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/projects/%s/serviceAccounts/%s/keys/%s", gcpIamURL, project, serviceAccountEmail, keyId)
	r := newDeleteServiceAccountKeyRequest(http.MethodDelete, url)
	e.addNewRequest(r)
	return r
}

func (e *expect) addNewRequest(r Request) {
	e.requests = append(e.requests, r)
}
