package gsm

import (
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"context"
	"encoding/json"
	"fmt"
	"github.com/broadinstitute/yale/internal/yale/logs"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type expectedRequest struct {
	requestMethod          string
	requestPath            string
	requestBodyMatcher     func(content []byte) (bool, error)
	requestQueryParameters map[string]string
	responseCode           int
	responseBody           []byte
}

// FakeGsmServer hand-rolled fake Google Secret Manager server that uses httptest and the GSM library's http support
// to intercept and fake requests. I really wish I could just generate a mock :'(
type FakeGsmServer struct {
	t                *testing.T
	expectedRequests []expectedRequest
	server           *httptest.Server
}

func (f *FakeGsmServer) ExpectListSecretWithNameFilter(project string, secret string, result *secretmanagerpb.Secret) {
	request := expectedRequest{
		requestMethod: "GET",
		requestPath:   fmt.Sprintf("/v1/projects/%s/secrets", project),
		requestQueryParameters: map[string]string{
			"filter": fmt.Sprintf("name:%s", secret),
		},
		responseCode: 200,
	}
	response := secretmanagerpb.ListSecretsResponse{
		Secrets: []*secretmanagerpb.Secret{},
	}
	if result != nil {
		response.Secrets = append(response.Secrets, result)
	}

	responseBody, err := json.Marshal(response)
	require.NoError(f.t, err)

	request.responseBody = responseBody

	f.expectedRequests = append(f.expectedRequests, request)
}

func (f *FakeGsmServer) ExpectCreateNewSecret(project string, secret string, requestMatcher func(*secretmanagerpb.Secret) bool, result *secretmanagerpb.Secret) {
	request := expectedRequest{
		requestMethod: "POST",
		requestPath:   fmt.Sprintf("/v1/projects/%s/secrets", project),
		responseCode:  201,
	}

	request.requestBodyMatcher = func(content []byte) (bool, error) {
		var r secretmanagerpb.Secret
		if err := json.Unmarshal(content, &r); err != nil {
			return false, fmt.Errorf("error unmarshalling request body to CreateSecretRequest: %v", err)
		}
		require.Equal(f.t, secret, r.Name, "expected secret.name to equal %s", secret)
		if requestMatcher == nil {
			return true, nil
		}
		return requestMatcher(&r), nil
	}

	responseBody, err := json.Marshal(result)
	require.NoError(f.t, err)
	request.responseBody = responseBody

	f.expectedRequests = append(f.expectedRequests, request)
}

func (f *FakeGsmServer) ExpectCreateNewSecretVersion(project string, secret string, payload []byte, result *secretmanagerpb.SecretVersion) {
	request := expectedRequest{
		requestMethod: "POST",
		requestPath:   fmt.Sprintf("/v1/projects/%s/secrets/%s:addVersion", project, secret),
		responseCode:  201,
	}

	request.requestBodyMatcher = func(content []byte) (bool, error) {
		var r secretmanagerpb.AddSecretVersionRequest
		if err := json.Unmarshal(content, &r); err != nil {
			return false, fmt.Errorf("error unmarshalling add secret version request: %v", err)
		}
		require.Equal(f.t, fmt.Sprintf("projects/%s/secrets/%s", project, secret), r.Parent)
		require.Equal(f.t, string(payload), string(r.GetPayload().GetData()))
		return true, nil
	}

	responseBody, err := json.Marshal(result)
	require.NoError(f.t, err)

	request.responseBody = responseBody

	f.expectedRequests = append(f.expectedRequests, request)
}

func (f *FakeGsmServer) Close() {
	f.server.Close()
}

func (f *FakeGsmServer) AssertExpectations() {
	require.Empty(f.t, f.expectedRequests, "%d unmet expectationts: %#v", len(f.expectedRequests), f.expectedRequests)
}

func (f *FakeGsmServer) NewClient() *secretmanager.Client {
	client, err := secretmanager.NewRESTClient(
		context.Background(),
		option.WithHTTPClient(f.server.Client()),
		option.WithEndpoint(f.server.URL),
	)
	require.NoError(f.t, err)
	return client
}

func (f *FakeGsmServer) httpHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logs.Info.Printf("received request: %s %s", r.Method, r.URL)

		if len(f.expectedRequests) == 0 {
			f.t.Errorf("received request %s %s, but no expectations are set", r.Method, r.URL.Path)
			f.t.FailNow()
		}

		nextRequest := f.expectedRequests[0]
		f.expectedRequests = f.expectedRequests[1:]

		require.Equal(f.t, nextRequest.requestMethod, r.Method, "expected request %v, got %s %s", nextRequest, r.Method, r.URL)
		require.Equal(f.t, nextRequest.requestPath, r.URL.Path, "expected request %v, got %s %s", nextRequest, r.Method, r.URL)

		if nextRequest.requestQueryParameters != nil {
			for name, value := range nextRequest.requestQueryParameters {
				actual := r.URL.Query().Get(name)
				logs.Debug.Printf("Matching query parameter %s (expect %s, got %s)", name, value, actual)
				require.Equal(f.t, value, actual, "expected request query parameter %q to have value %s, got %s", name, value, actual)
			}
		}

		if nextRequest.requestBodyMatcher != nil {
			body, err := io.ReadAll(r.Body)
			require.NoError(f.t, err, "fake gsm server: error reading request body", string(body))
			logs.Debug.Printf("Matching request body: %s", string(body))
			matches, err := nextRequest.requestBodyMatcher(body)
			require.NoError(f.t, err, "error matching request body", string(body))
			require.True(f.t, matches, "request body did not match expectation: %s", string(body))
		}

		logs.Info.Printf("writing %d response", nextRequest.responseCode)
		w.WriteHeader(nextRequest.responseCode)
		if _, err := w.Write(nextRequest.responseBody); err != nil {
			f.t.Errorf("error writing response body: %v", err)
			f.t.FailNow()
		}
	})
}

func NewFakeGsm(t *testing.T) *FakeGsmServer {
	fakeGsm := &FakeGsmServer{
		t: t,
	}

	server := httptest.NewServer(fakeGsm.httpHandler())
	fakeGsm.server = server
	return fakeGsm
}
