package gcp

import (
	"google.golang.org/api/policyanalyzer/v1"
)
type ActivityResp struct {
	Activities   []policyanalyzer.GoogleCloudPolicyanalyzerV1Activity
}
// Query key
type CreateQuery interface {
	With (key string) CreateQuery
	Returns(key policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse, err error) CreateQuery
	Request
}

type createQuery struct {
	request
}

func newQueryRequest(method string, query string) CreateQuery {
	return &createQuery{
		request: *newRequest(method, query),
	}
}

func (r *createQuery) With(query string) CreateQuery {
	r.ResponseBody(query)
	return r
}


func (r *createQuery) Returns(activitiesResponse policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse, err error) CreateQuery {
	if err != nil{
		r.status = 400
		r.ResponseBody(nil)
	} else{
		r.ResponseBody(activitiesResponse)
	}
	return r
}

//// get key
type FilterRequest interface {
	With(filter string) FilterRequest
	Returns(activities policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse, err error) FilterRequest
	Request
}

type filterRequest struct {
	request
}


func (r *filterRequest) With(filter string) FilterRequest {
	r.RequestBody(filter)
	return r
}

func (r *filterRequest) Returns(activitiesResponse policyanalyzer.GoogleCloudPolicyanalyzerV1QueryActivityResponse,  err error) FilterRequest {
	if err != nil{
		r.status = 400
		r.ResponseBody(nil)
	} else{
		r.ResponseBody(activitiesResponse)
	}
	return r
}
func newFilterRequest(method string, filter string) FilterRequest {
	return &filterRequest{
		request: *newRequest(method, filter),
	}
}
