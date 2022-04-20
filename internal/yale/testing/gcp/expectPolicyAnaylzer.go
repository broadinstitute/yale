package gcp

import "fmt"

// ExpectIam is an interface for setting expectations on a mock iam.Service
type ExpectPolicyAnalyzer interface {
	// CreateQuery configures the mock to expect a request to query policy activities
	CreateQuery(query string) CreateQuery
	FilterRequest(filter string) FilterRequest
}

func newExpectPolicyAnalyzer() *expectPolicyAnalyzer {
	return &expectPolicyAnalyzer{}
}

// implements the ExpectIam interface
type expectPolicyAnalyzer struct {
	requests []Request
}

// CreateQuery
// see https://cloud.google.com/iam/docs/reference/rest/v1/projects.serviceAccounts.keys/create
func (e *expectPolicyAnalyzer) CreateQuery(googleProject string) CreateQuery {
	query := fmt.Sprintf("projects/%s/locations/us-central1-a/activityTypes/serviceAccountKeyLastAuthentication", googleProject)
	r := newQueryRequest(methodPost, query)
	e.addNewRequest(r)
	return r
}

func (e *expectPolicyAnalyzer)FilterRequest(keyName string) FilterRequest {
	filter := fmt.Sprintf("activities.fullResourceName = \"//iam.googleapis.com/%s\"", keyName)
	r := newFilterRequest(methodPost, filter)
	e.addNewRequest(r)
	return r
}
func (e *expectPolicyAnalyzer) addNewRequest(r Request) {
	e.requests = append(e.requests, r)
}