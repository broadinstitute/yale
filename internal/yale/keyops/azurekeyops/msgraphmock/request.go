package msgraphmock

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/google/go-cmp/cmp"
	"github.com/jarcoal/httpmock"
)

// Request encapsulates a mocked MS Graph API request & response
type Request interface {
	RequestBody(requestBody interface{}) Request

	Status(status int)

	CallCount(callcount int)

	Error(error)

	ResponseBody(responseBody interface{}) Request

	Times(callCount int) Request

	getMethod() string

	getUrl() string

	getCallCount() int

	buildResponder() httpmock.Responder
}

type request struct {
	method       string
	url          string
	requestBody  interface{}
	error        error
	status       int
	responseBody interface{}
	callCount    int
}

func (r *request) Error(error error) {
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

// getMethod returns the method
func (r *request) getMethod() string {
	return r.method
}

// getUrl returns the url
func (r *request) getUrl() string {
	return r.url
}

// getCallCount returns the call count
func (r *request) getCallCount() int {
	return r.callCount
}

// buildResponder returns an httpmock.Responder for this request
func (r *request) buildResponder() httpmock.Responder {
	switch r.method {
	case http.MethodPost:
		return buildPostResponder(r)
	default:
		return buildGetResponder(r)
	}
}

func buildGetResponder(r *request) httpmock.Responder {
	status := r.status
	if status == 0 {
		status = http.StatusOK
	}

	return func(req *http.Request) (*http.Response, error) {
		return httpmock.NewJsonResponse(status, r.responseBody)
	}
}

func buildPostResponder(r *request) httpmock.Responder {
	if r.method != http.MethodPost {
		panic("buildPostResponder called for non-POST request")
	}

	status := r.status
	if status == 0 {
		status = http.StatusCreated
	}

	if r.responseBody == nil {
		panic(fmt.Errorf("please configure a response body for %s %s", r.method, r.url))
	}

	return func(req *http.Request) (*http.Response, error) {
		var expected, actual map[string]interface{}

		expectedBytes, err := json.Marshal(r.requestBody)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(expectedBytes, &expected); err != nil {
			return nil, err
		}

		actualBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(actualBytes, &actual); err != nil {
			return nil, err
		}

		if diff := cmp.Diff(expected, actual); diff != "" {
			return nil, fmt.Errorf("POST %s\n\t%T differ (-got, +want):\n%s", r.url, r.requestBody, diff)
		}

		if status != http.StatusCreated {
			return nil, errors.New("POST request failed")
		}

		return httpmock.NewJsonResponse(status, r.responseBody)
	}
}
