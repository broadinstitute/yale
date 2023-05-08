package main

import (
	monitoring "cloud.google.com/go/monitoring/apiv3"
	"context"
	"encoding/json"
	"github.com/google/go-replayers/grpcreplay"
	"github.com/google/go-replayers/httpreplay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	"net/http"
	"os"
	"testing"
	"time"
)

// set to true to record new test data (make sure you have application default credentials with permission
// to read monitoring/iam data from broad-dsde-dev)
const recordMode = false

// recorded input files
// note that grpc uses tls for auth, and the http replay library automatically
// strips out Authorization headers, so we don't need to add any special handling to remove
// credentials
const grpcFile = "testdata/recordings/authmetrics.grpc"
const httpFile = "testdata/recordings/authmetrics.http"
const metadataFile = "testdata/recordings/authmetrics.timestamp"

type testMetadata struct {
	Timestamp time.Time
}

func Test_Record(t *testing.T) {
	am := newAuthMetrics(t, recordMode)

	// in an ideal world, we'd issue new keys for this test and add better timestamp handling,
	// but for now we accept that we'll have to update this test whenever it is re-recorded,
	// which should be rarely.

	lastAuth, err := am.LastAuthTime("broad-dsde-dev", "cromwell-carbonite-user@broad-dsde-dev.iam.gserviceaccount.com", "2ac28ba60e2683441fa01ba0909f814560f5f02a")
	require.NoError(t, err)
	require.NotNil(t, lastAuth)
	assert.Equal(t, "2023-05-07 20:50:00 +0000 UTC", (*lastAuth).String(), "cromwell-carbonite-user")

	lastAuth, err = am.LastAuthTime("broad-dsde-dev", "externalcreds-dev@broad-dsde-dev.iam.gserviceaccount.com", "4ef71a1def10dcaa252f0e17de599e28823c272b")
	require.NoError(t, err)
	require.NotNil(t, lastAuth)
	assert.Equal(t, "2023-05-02 16:20:00 +0000 UTC", (*lastAuth).String(), "externalcreds-dev")

	lastAuth, err = am.LastAuthTime("broad-dsde-dev", "dev-ci-sa@broad-dsde-dev.iam.gserviceaccount.com", "8276c6e9f5cfd3d4d9aeb11699bcca77ee49858d")
	require.NoError(t, err)
	require.NotNil(t, lastAuth)
	assert.Equal(t, "2023-05-06 13:50:00 +0000 UTC", (*lastAuth).String(), "dev-ci-sa")

	lastAuth, err = am.LastAuthTime("broad-dsde-dev", "drshub-dev@broad-dsde-dev.iam.gserviceaccount.com", "df69bf61b5215bd2ab9194d52f95bc09a3a4877b")
	require.NoError(t, err)
	require.NotNil(t, lastAuth)
	assert.Equal(t, "2023-05-08 13:50:00 +0000 UTC", (*lastAuth).String(), "drshub-dev")

	lastAuth, err = am.LastAuthTime("broad-dsde-dev", "drshub-dev@broad-dsde-dev.iam.gserviceaccount.com", "key-does-not-exist")
	require.NoError(t, err)
	assert.Nil(t, lastAuth)
}

func newAuthMetrics(t *testing.T, recordMode bool) *authMetrics {
	if recordMode {
		return newAuthMetricsInRecordMode(t)
	} else {
		return newAuthMetricsInReplayMode(t)
	}
}

func newAuthMetricsInRecordMode(t *testing.T) *authMetrics {
	metadata := testMetadata{
		Timestamp: time.Now(),
	}
	t.Cleanup(func() {
		if err := writeMetadata(metadata); err != nil {
			t.Fatal(err)
		}
	})

	grpcRecorder, err := grpcreplay.NewRecorder(grpcFile, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := grpcRecorder.Close(); err != nil {
			t.Fatal(err)
		}
	})

	var options []option.ClientOption
	for _, o := range grpcRecorder.DialOptions() {
		options = append(options, option.WithGRPCDialOption(o))
	}

	// create new http recorder
	httpRecorder, err := httpreplay.NewRecorder(httpFile, nil)
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

	return buildAuthMetrics(t, metadata, httpClient, options...)
}

func newAuthMetricsInReplayMode(t *testing.T) *authMetrics {
	meta := readMetadata(t)

	grpcReplayer, err := grpcreplay.NewReplayer(grpcFile, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := grpcReplayer.Close(); err != nil {
			t.Fatal(err)
		}
	})
	conn, err := grpcReplayer.Connection()
	require.NoError(t, err)

	httpReplayer, err := httpreplay.NewReplayer(httpFile)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := httpReplayer.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return buildAuthMetrics(t, meta, httpReplayer.Client(), option.WithGRPCConn(conn))
}

// build newAuthMetrics given replayer/recorder constructors for http and grpc protocols
func buildAuthMetrics(t *testing.T, metadata testMetadata, httpClient *http.Client, grpcOpts ...option.ClientOption) *authMetrics {
	iamService, err := iam.NewService(context.Background(), option.WithHTTPClient(httpClient))
	require.NoError(t, err)

	metricClient, err := monitoring.NewMetricClient(context.Background(), grpcOpts...)
	require.NoError(t, err)

	return newWithClients(metricClient, iamService, metadata.Timestamp)
}

func readMetadata(t *testing.T) testMetadata {
	data, err := os.ReadFile(metadataFile)
	require.NoError(t, err)

	var meta testMetadata
	err = json.Unmarshal(data, &meta)
	require.NoError(t, err)

	return meta
}

func writeMetadata(meta testMetadata) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	err = os.WriteFile(metadataFile, data, 0600)
	if err != nil {
		return err
	}
	return nil
}
