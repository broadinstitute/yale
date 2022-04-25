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
