package gsm

import (
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"context"
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
	requestBody            []byte
	requestQueryParameters map[string]string
	responseCode           int
	responseBody           []byte
}

type FakeGsmServer struct {
	t                *testing.T
	expectedRequests []expectedRequest
	server           *httptest.Server
}

func (f *FakeGsmServer) ExpectListSecret(project string, name string) {

}

func (f *FakeGsmServer) ExpectCreateNewSecret(project string, name string) {

}

func (f *FakeGsmServer) ExpectCreateNewSecretVersion(project string, name string, payload []byte) {

}

func (f *FakeGsmServer) Close() {
	f.server.Close()
}

func (f *FakeGsmServer) NewClient() *secretmanager.Client {
	client, err := secretmanager.NewRESTClient(
		context.Background(),
		option.WithHTTPClient(f.server.Client()),
	)
	require.NoError(f.t, err)
	return client
}

func (f *FakeGsmServer) httpHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logs.Info.Printf("received request: %s %s", r.Method, r.URL.Path)

		if len(f.expectedRequests) == 0 {
			f.t.Errorf("received request %s %s, but no expectations are set", r.Method, r.URL.Path)
			f.t.FailNow()
		}

		nextRequest := f.expectedRequests[0]
		f.expectedRequests = f.expectedRequests[1:]

		require.Equal(f.t, nextRequest.requestMethod, r.Method, "expected request %v, got %s %s", nextRequest, r.Method, r.URL)
		require.Equal(f.t, nextRequest.requestPath, r.URL.Path, "expected request %v, got %s %s", nextRequest, r.Method, r.URL)

		if len(nextRequest.requestBody) != 0 {
			body, err := io.ReadAll(r.Body)
			require.NoError(f.t, err, "fake gsm server: error reading request body")
			require.Equal(f.t, string(nextRequest.responseBody), string(body), "request body does not match expectation")
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
