package msgraphmock

import (
	"github.com/manicminer/hamilton/msgraph"
)

type AddPasswordRequest interface {
	With(passwordCredential msgraph.PasswordCredential) AddPasswordRequest
	Returns(key *msgraph.PasswordCredential) AddPasswordRequest
	Request
}

type addPasswordRequest struct {
	request
}

func newAddPasswordRequest(method string, url string) AddPasswordRequest {
	return &addPasswordRequest{
		request: *newRequest(method, url),
	}
}

func (r *addPasswordRequest) With(passwordCredential msgraph.PasswordCredential) AddPasswordRequest {
	body := make(map[string]interface{})
	body["passwordCredential"] = passwordCredential
	r.RequestBody(body)
	return r
}

func (r *addPasswordRequest) Returns(key *msgraph.PasswordCredential) AddPasswordRequest {
	r.ResponseBody(key)
	return r
}

type GetApplicationRequest interface {
	Returns(application *msgraph.Application) GetApplicationRequest
	Request
}

type getApplicationRequest struct {
	request
}

func newGetApplicationRequest(method string, url string) GetApplicationRequest {
	return &getApplicationRequest{
		request: *newRequest(method, url),
	}
}

func (r *getApplicationRequest) Returns(application *msgraph.Application) GetApplicationRequest {
	r.ResponseBody(application)
	return r
}

type RemovePasswordRequest interface {
	With(keyId string) RemovePasswordRequest
	Returns() RemovePasswordRequest
	Request
}

type removePasswordRequest struct {
	request
}

func newRemovePasswordRequest(method string, url string) RemovePasswordRequest {
	return &removePasswordRequest{
		request: *newRequest(method, url),
	}
}

func (r *removePasswordRequest) With(keyId string) RemovePasswordRequest {
	body := make(map[string]interface{})
	body["keyId"] = keyId
	r.RequestBody(body)
	return r
}

func (r *removePasswordRequest) Returns() RemovePasswordRequest {
	r.ResponseBody(struct{}{})
	return r
}
