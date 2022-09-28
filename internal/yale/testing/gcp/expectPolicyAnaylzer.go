package gcp

import (
	"fmt"
	"google.golang.org/api/googleapi"
)

const gcpPolicyAnalyzerURL = "https://policyanalyzer.googleapis.com/v1"

// ExpectPolicyAnalyzer is an interface for setting expectations on a mock iam.Service
type ExpectPolicyAnalyzer interface {
	// CreateQuery configures the mock to expect a request to query policy activities
	CreateQuery(query string, statusCode int, callCount int) CreateQuery
}

func newExpectPolicyAnalyzer() *expectPolicyAnalyzer {
	return &expectPolicyAnalyzer{}
}

// implements the ExpectPolicyAnalyzer interface
type expectPolicyAnalyzer struct {
	requests []Request
}

// CreateQuery
// see https://cloud.google.com/iam/docs/reference/policyanalyzer/rest/v1/projects.locations.activityTypes.activities/query
func (e *expectPolicyAnalyzer) CreateQuery(googleProject string, statusCode int, errorMsg string, callCount int) CreateQuery {
	query := fmt.Sprintf("%s/projects/%s/locations/us-central1-a/activityTypes/serviceAccountKeyLastAuthentication/activities:query", gcpPolicyAnalyzerURL, googleProject)
	error := googleapi.Error{Message: errorMsg}
	r := newQueryRequest(methodGet, query)
	r.ErrorMsg(error)
	r.Status(statusCode)
	r.CallCount(callCount)
	e.addNewRequest(r)
	return r
}

func (e *expectPolicyAnalyzer) addNewRequest(r Request) {
	e.requests = append(e.requests, r)
}
