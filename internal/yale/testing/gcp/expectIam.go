package gcp

import (
	"fmt"
	"net/http"
)

const gcpIamURL = "https://iam.googleapis.com/v1"

// ExpectIam is an interface for setting expectations on a mock iam.Service
type ExpectIam interface {
	// CreateServiceAccountKey configures the mock to expect a request to create a service account key
	CreateServiceAccountKey(project string, serviceAccountEmail string, hasError bool) CreateServiceAccountKeyRequest
	// GetServiceAccountKey configures the mock to expect a request to get a service account key
	GetServiceAccountKey(keyName string, hasError bool) GetServiceAccountKeyRequest
	// DisableServiceAccountKey configures the mock to expect a request to disable a service account key
	DisableServiceAccountKey(keyName string) DisableServiceAccountKeyRequest
	// DeleteServiceAccountKey configures the mock to expect a request that deletes a service account key
	DeleteServiceAccountKey(keyName string, hasError bool) DeleteServiceAccountKeyRequest
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
func (e *expectIam) CreateServiceAccountKey(project string, serviceAccountEmail string, hasError bool) CreateServiceAccountKeyRequest {
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
func (e *expectIam) GetServiceAccountKey(keyName string, hasError bool) GetServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/%s", gcpIamURL, keyName)
	r := newGetServiceAccountKeyRequest(methodGet, url)
	if hasError {
		r.Status(400)
	}
	e.addNewRequest(r)
	return r
}

// DisableServiceAccountKey
// see https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/disable
func (e *expectIam) DisableServiceAccountKey(keyName string) DisableServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/%s:disable", gcpIamURL, keyName)
	r := createDisableServiceAccountKeyRequest(methodPost, url)
	e.addNewRequest(r)
	return r
}

// DeleteServiceAccountKey
// see https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/delete
func (e *expectIam) DeleteServiceAccountKey(keyName string, hasError bool) DeleteServiceAccountKeyRequest {
	url := fmt.Sprintf("%s/%s", gcpIamURL, keyName)
	r := createDeleteServiceAccountKeyRequest(http.MethodDelete, url)
	if hasError {
		r.Status(400)
	}
	e.addNewRequest(r)
	return r
}
func (e *expectIam) addNewRequest(r Request) {
	e.requests = append(e.requests, r)
}
