package gcp

import (
	"fmt"
)

const gcpIamURL = "https://iam.googleapis.com/v1"

// ExpectIam is an interface for setting expectations on a mock iam.Service
type ExpectIam interface {
	// CreateServiceAccountKey configures the mock to expect a request to create a service account key
	CreateServiceAccountKey(project string, serviceAccountEmail string) CreateServiceAccountKeyRequest
	// GetServiceAccountKey configures the mock to expect a request to get a service account key
	GetServiceAccountKey(project string, serviceAccountEmail string, keyName string) GetServiceAccountKeyRequest
	DisableServiceAccountKey(project string, serviceAccountEmail string, keyName string) DisableServiceAccountKeyRequest
}

func newExpectIam() *expectIam {
	return &expectIam{}
}

// implements the ExpectIam interface
type expectIam struct {
	requests []Request
}

// CreateServiceAccountKey
// see https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/create
func (e *expectIam) CreateServiceAccountKey(project string, serviceAccountEmail string) CreateServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/projects/%s/serviceAccounts/%s/keys", gcpIamURL, project, serviceAccountEmail)
	r := newCreateServiceAccountKeyRequest(methodPost, url)
	e.addNewRequest(r)
	return r
}

// GetServiceAccountKey
// see https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/get
func (e *expectIam) GetServiceAccountKey(project string, serviceAccountEmail string, keyName string) GetServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/projects/%s/serviceAccounts/%s/keys/%s", gcpIamURL, project, serviceAccountEmail, keyName)
	r := newGetServiceAccountKeyRequest(methodGet, url)
	return r
}

// GetServiceAccountKey
// see https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/disable
func (e *expectIam) DisableServiceAccountKey(project string, serviceAccountEmail string, keyName string) DisableServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/projects/%s/serviceAccounts/%s/keys/%s", gcpIamURL, project, serviceAccountEmail, keyName)
	r := createDisableServiceAccountKeyRequest(methodPost, url)
	return r
}
func (e *expectIam) addNewRequest(r Request) {
	e.requests = append(e.requests, r)
}
