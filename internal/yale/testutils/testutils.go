package testutils

import (
	"context"
	"fmt"
	"github.com/google/go-replayers/httpreplay"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	"net/http"
	"testing"
)

func NewRecordingHTTPClientWithADCAuth(t *testing.T, recordFile string) *http.Client {
	// create new http recorder
	httpRecorder, err := httpreplay.NewRecorder(recordFile, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := httpRecorder.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// add Google ADC auth to the http client
	tokenSource, err := google.DefaultTokenSource(context.Background())
	require.NoError(t, err)

	httpClient := httpRecorder.Client()
	httpClient.Transport = &oauth2.Transport{
		Source: tokenSource,
		Base:   httpClient.Transport,
	}
	return httpClient
}

func NewReplayingHTTPClient(t *testing.T, replayFile string) *http.Client {
	httpReplayer, err := httpreplay.NewReplayer(replayFile)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := httpReplayer.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return httpReplayer.Client()
}

func NewFakeK8sClient(t *testing.T, objects ...runtime.Object) kubernetes.Interface {
	k8s := k8sfake.NewSimpleClientset(objects...)
	k8s.PrependReactor("create", "secrets", secretDataReactor)
	return k8s
}

// secretDataReactor: A reactor that makes persists secret StringData updates to the fake cluster
// yanked from: https://github.com/creydr/go-k8s-utils
func secretDataReactor(action ktesting.Action) (bool, runtime.Object, error) {
	secret, ok := action.(ktesting.CreateAction).GetObject().(*corev1.Secret)
	if !ok {
		return false, nil, fmt.Errorf("SecretDataReactor can only be applied on secrets")
	}

	if len(secret.StringData) > 0 {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}

		for k, v := range secret.StringData {
			secret.Data[k] = []byte(v)
		}
	}

	return false, nil, nil
}
