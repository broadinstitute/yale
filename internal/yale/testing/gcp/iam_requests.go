package gcp

import "google.golang.org/api/iam/v1"

// create key
type CreateServiceAccountKeyRequest interface {
	With(keyRequest iam.CreateServiceAccountKeyRequest) CreateServiceAccountKeyRequest
	Returns(key iam.ServiceAccountKey) CreateServiceAccountKeyRequest
	Request
}

type createServiceAccountKeyRequest struct {
	request
}

func newCreateServiceAccountKeyRequest(method string, url string) CreateServiceAccountKeyRequest {
	return &createServiceAccountKeyRequest{
		request: *newRequest(method, url),
	}
}

func (r *createServiceAccountKeyRequest) With(keyRequest iam.CreateServiceAccountKeyRequest) CreateServiceAccountKeyRequest {
	r.RequestBody(keyRequest)
	return r
}

func (r *createServiceAccountKeyRequest) Returns(key iam.ServiceAccountKey) CreateServiceAccountKeyRequest {
	r.ResponseBody(key)
	return r
}

// get key
type GetServiceAccountKeyRequest interface {
	Returns(key iam.ServiceAccountKey) GetServiceAccountKeyRequest
	Request
}

type getServiceAccountKeyRequest struct {
	request
}

func newGetServiceAccountKeyRequest(method string, url string) GetServiceAccountKeyRequest {
	return &getServiceAccountKeyRequest{
		request: *newRequest(method, url),
	}
}

func (r *getServiceAccountKeyRequest) Returns(key iam.ServiceAccountKey) GetServiceAccountKeyRequest {
	r.ResponseBody(key)
	return r
}

//Disable key
type DisableServiceAccountKeyRequest interface {
	With(keyRequest iam.DisableServiceAccountKeyRequest) DisableServiceAccountKeyRequest
	Returns() DisableServiceAccountKeyRequest
	Request
}

type disableServiceAccountKeyRequest struct {
	request
}

func (r *disableServiceAccountKeyRequest) With(keyRequest iam.DisableServiceAccountKeyRequest) DisableServiceAccountKeyRequest {
	r.RequestBody(keyRequest)
	return r
}

// https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/disable#response-bodyhttps://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/disable#response-body
func (r *disableServiceAccountKeyRequest) Returns() DisableServiceAccountKeyRequest {
	r.ResponseBody(struct {
	}{})
	return r
}

func createDisableServiceAccountKeyRequest(method string, url string) DisableServiceAccountKeyRequest {
	return &disableServiceAccountKeyRequest{
		request: *newRequest(method, url),
	}
}

// Delete key
type DeleteServiceAccountKeyRequest interface {
	Returns() DeleteServiceAccountKeyRequest
	Request
}

type deleteServiceAccountKeyRequest struct {
	request
}
func createDeleteServiceAccountKeyRequest(method string, url string) DeleteServiceAccountKeyRequest {
	return &deleteServiceAccountKeyRequest{
		request: *newRequest(method, url),
	}
}

// https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/disable#response-bodyhttps://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/disable#response-body
func (r *deleteServiceAccountKeyRequest) Returns() DeleteServiceAccountKeyRequest {
	r.ResponseBody(struct {
	}{})
	return r
}