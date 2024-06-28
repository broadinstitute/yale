package github

import (
	"github.com/google/go-github/v62/github"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v3/cassette"
	"gopkg.in/dnaeon/go-vcr.v3/recorder"
	"strings"
	"testing"
)

const githubToken = "<add your PAT here while recording>"
const repoName = "broadinstitute/yale"
const secretName = "MY_TEST_SECRET"

// this is record-playback style test and can be used to verify that GitHub API calls are being
// made as expected.

func Test_Client_WritesSecret(t *testing.T) {
	tokens := strings.SplitN(repoName, "/", 2)
	repo, org := tokens[0], tokens[1]

	r, err := recorder.NewWithOptions(&recorder.Options{
		CassetteName:       "testdata/fixtures/client_test",
		Mode:               recorder.ModeRecordOnce, // change or delete fixture to re-record
		SkipRequestLatency: true,
	})
	require.NoError(t, err)

	// stop recording when the test finishes
	defer func() {
		require.NoError(t, r.Stop())
	}()

	// remove Auth header before saving recorded HTTP interaction
	r.AddHook(func(i *cassette.Interaction) error {
		delete(i.Request.Headers, "Authorization")
		return nil
	}, recorder.AfterCaptureHook)

	// supply the recorder's http client to github client and wrap in our own Client interface
	httpClient := r.GetDefaultClient()
	githubClient := github.NewClient(httpClient).WithAuthToken(githubToken)
	_client := NewClient(githubClient)

	// write the secret
	require.NoError(t, _client.WriteSecret(repo, org, secretName, []byte("some data")))
}
