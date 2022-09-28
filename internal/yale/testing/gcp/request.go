package gcp

import (
	"encoding/json"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/jarcoal/httpmock"
	"github.com/pkg/errors"
	"google.golang.org/api/googleapi"
	"io/ioutil"
	"net/http"
)

const methodGet = "GET"
const methodPost = "POST"

const defaultGetStatus = 200
const defaultPostStatus = 201
const defaultCallCount = 1

// Request encapsulates a mocked GCP API request & response
type Request interface {
	// RequestBody requires that any requests matching the url pattern have the given response body (marshalled to JSON)
	RequestBody(requestBody interface{}) Request
	// Status sets status for the mock response (default 200 for requests without a body, 201 for requests with a body)
	Status(status int)
	// CallCount sets the expected about of calls for the mocked request (
	CallCount(callcount int)
	// CallCount sets the expected about of calls for the mocked request (
	Error(error googleapi.Error)
	// ResponseBody sets the body for the mock response (marshalled to JSON)
	ResponseBody(responseBody interface{}) Request
	// Times sets the number of times this request should be expected
	Times(callCount int) Request
	// getMethod returns the method
	getMethod() string
	// getUrl returns the url
	getUrl() string
	// getCallCount returns the call count
	getCallCount() int
	// buildResponder returns an httpmock.Responder for this request
	buildResponder() httpmock.Responder
}

// request implements Request interface
type request struct {
	method       string
	url          string
	requestBody  interface{}
	error        googleapi.Error
	status       int
	responseBody interface{}
	callCount    int
}

func (r *request) Error(error googleapi.Error) {
	r.error = error
}

func (r *request) CallCount(callcount int) {
	r.callCount = callcount
}

func newRequest(method string, url string) *request {
	return &request{
		method:    method,
		url:       url,
		callCount: 1,
	}
}

// RequestBody verifies that the request has the given request body (marshalled to JSON)
func (r *request) RequestBody(requestBody interface{}) Request {
	r.requestBody = requestBody
	return r
}

// Status sets status for the mock response (default 200 for requests without a body, 201 for requests with a body)
func (r *request) Status(status int) {
	r.status = status
}

// ResponseBody sets the body for the mock response (marshalled to JSON)
func (r *request) ResponseBody(responseBody interface{}) Request {
	r.responseBody = responseBody
	return r
}

// Times sets the number of times this request should be expected
func (r *request) Times(callCount int) Request {
	r.callCount = callCount
	return r
}

func (r *request) buildResponder() httpmock.Responder {
	switch r.method {
	case methodPost:
		return buildPostResponder(r)
	default:
		return buildResponder(r)
	}
}

// Creates a simple responder for requests
func buildResponder(r *request) httpmock.Responder {
	status := r.status
	if status == 0 {
		status = defaultGetStatus
	}
	return func(req *http.Request) (*http.Response, error) {
		//if status != defaultGetStatus {
		//	return nil, errors.New("The request has thrown an error.")
		//}
		return httpmock.NewJsonResponse(status, r.responseBody)
	}

}

// Creates an httpmock.Responder for POST requests that validates the actual request body matches the expected request body
func buildPostResponder(r *request) httpmock.Responder {
	if r.method != methodPost {
		panic("this function should only be called for post requests")
	}

	status := r.status
	callcount := r.callCount
	if status == 0 {
		status = defaultPostStatus
	}
	if callcount == 0 {
		callcount = defaultCallCount
	}

	if r.responseBody == nil {
		panic(fmt.Errorf("please configure a response body for %v %v", r.method, r.url))
	}

	return func(req *http.Request) (*http.Response, error) {
		// We need to compare the _expected_ request body with the _actual_ request body,
		// to make sure we're sending the right API calls to GCP.
		//
		// Since the _actual_ request body is passed to us as a JSON string, but callers of this method
		// should pass in the _expected_ request body as a GCP client struct like
		// `compute.RegionDisksAddResourcePoliciesRequest`, we convert both to map[string]interface{} and compare
		// then with cmp.diff(). To do this, we marshal the expected struct to JSON and then unmarshal it back to
		// map[string]interface{}.
		//
		// A Go expert might be able to do something fancier with reflection, in the mean time this gets the job done :)
		var expected, actual map[string]interface{}

		expectedBytes, err := json.Marshal(r.requestBody)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(expectedBytes, &expected); err != nil {
			return nil, err
		}

		actualBytes, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(actualBytes, &actual); err != nil {
			return nil, err
		}

		if diff := cmp.Diff(expected, actual); diff != "" {
			return nil, fmt.Errorf("POST %s\n\t%T differ (-got, +want):\n%s", r.url, r.requestBody, diff)
		}
		if status != defaultPostStatus {
			return nil, errors.New("The request has thrown an error.")
		}
		return httpmock.NewJsonResponse(status, r.responseBody)
	}
}

func (r *request) getMethod() string {
	return r.method
}

func (r *request) getUrl() string {
	return r.url
}

func (r *request) getCallCount() int {
	return r.callCount
}
