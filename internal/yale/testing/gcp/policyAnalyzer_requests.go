package gcp

import (
	"google.golang.org/api/googleapi"
	"google.golang.org/api/policyanalyzer/v1"
)

type ActivityResp struct {
	Activities []policyanalyzer.GoogleCloudPolicyanalyzerV1Activity
}

// Query key
type CreateQuery interface {
	Returns(key policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse) CreateQuery
	Request
}

type createQuery struct {
	request
}

func (r *createQuery) CallCount(callcount int) {
	r.callCount = callcount
}

func (r *createQuery) Error(err *googleapi.Error) {
	r.error = err
}
func newQueryRequest(method string, query string) CreateQuery {
	return &createQuery{
		request: *newRequest(method, query),
	}
}

func (r *createQuery) Returns(activitiesResponse policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse) CreateQuery {
	r.ResponseBody(activitiesResponse)
	return r
}
