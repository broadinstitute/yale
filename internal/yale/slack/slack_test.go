package slack

import (
	"testing"

	"github.com/broadinstitute/yale/internal/yale/cache"
	"github.com/slack-go/slack"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const postWebhookMethod = "PostWebhook"

func Test_SlackNotifier_KeyIssued(t *testing.T) {
	client := newMockClient(t)

	s := &slackNotifier{
		client: client,
	}

	client.On(
		postWebhookMethod,
		&slack.WebhookMessage{
			Attachments: []slack.Attachment{
				{
					Color:     okColor,
					Title:     "GcpSaKey Issued",
					TitleLink: "https://console.cloud.google.com/iam-admin/serviceaccounts/details/sa1@p.com?project=p",
					Text:      "A new <https://console.cloud.google.com/iam-admin/serviceaccounts/details/sa1@p.com?project=p|GcpSaKey> was issued in `p`",
					Fields: []slack.AttachmentField{
						{
							Title: "Email",
							Value: "sa1@p.com",
						}, {
							Title: "Key ID",
							Value: "`1234`",
						},
					},
				},
			},
		},
	).Return(nil)

	require.NoError(t, s.KeyIssued(&cache.Entry{
		Type: cache.GcpSaKey,
		Identifier: cache.GcpSaKeyEntryIdentifier{
			Email:   "sa1@p.com",
			Project: "p",
		},
	}, "1234"))
}

func Test_SlackNotifier_KeyDisabled(t *testing.T) {
	client := newMockClient(t)

	s := &slackNotifier{
		client: client,
	}

	client.On(
		postWebhookMethod,
		&slack.WebhookMessage{
			Attachments: []slack.Attachment{
				{
					Color:     okColor,
					Title:     "GcpSaKey Disabled",
					TitleLink: "https://console.cloud.google.com/iam-admin/serviceaccounts/details/sa1@p.com?project=p",
					Text:      "A <https://console.cloud.google.com/iam-admin/serviceaccounts/details/sa1@p.com?project=p|GcpSaKey> was disabled in `p`",
					Fields: []slack.AttachmentField{
						{
							Title: "Email",
							Value: "sa1@p.com",
						}, {
							Title: "Key ID",
							Value: "`1234`",
						},
					},
				},
			},
		},
	).Return(nil)

	require.NoError(t, s.KeyDisabled(&cache.Entry{
		Type: cache.GcpSaKey,
		Identifier: cache.GcpSaKeyEntryIdentifier{
			Email:   "sa1@p.com",
			Project: "p",
		},
	}, "1234"))
}

func Test_SlackNotifier_KeyDeleted(t *testing.T) {
	client := newMockClient(t)

	s := &slackNotifier{
		client: client,
	}

	client.On(
		postWebhookMethod,
		&slack.WebhookMessage{
			Attachments: []slack.Attachment{
				{
					Color:     okColor,
					Title:     "GcpSaKey Deleted",
					TitleLink: "https://console.cloud.google.com/iam-admin/serviceaccounts/details/sa1@p.com?project=p",
					Text:      "A <https://console.cloud.google.com/iam-admin/serviceaccounts/details/sa1@p.com?project=p|GcpSaKey> was deleted in `p`",
					Fields: []slack.AttachmentField{
						{
							Title: "Email",
							Value: "sa1@p.com",
						}, {
							Title: "Key ID",
							Value: "`1234`",
						},
					},
				},
			},
		},
	).Return(nil)

	require.NoError(t, s.KeyDeleted(&cache.Entry{
		Type: cache.GcpSaKey,
		Identifier: cache.GcpSaKeyEntryIdentifier{
			Email:   "sa1@p.com",
			Project: "p",
		},
	}, "1234"))
}

func Test_SlackNotifier_Error(t *testing.T) {
	client := newMockClient(t)

	s := &slackNotifier{
		client: client,
	}

	client.On(
		postWebhookMethod,
		&slack.WebhookMessage{
			Attachments: []slack.Attachment{
				{
					Color:     errorColor,
					Title:     "Error",
					TitleLink: "https://console.cloud.google.com/iam-admin/serviceaccounts/details/sa1@p.com?project=p",
					Text:      "Error processing <https://console.cloud.google.com/iam-admin/serviceaccounts/details/sa1@p.com?project=p|GcpSaKey> in `p`",
					Fields: []slack.AttachmentField{
						{
							Title: "Email",
							Value: "sa1@p.com",
						}, {
							Title: "Error",
							Value: "something went wrong",
						},
					},
				},
			},
		},
	).Return(nil)

	require.NoError(t, s.Error(&cache.Entry{
		Type: cache.GcpSaKey,
		Identifier: cache.GcpSaKeyEntryIdentifier{
			Email:   "sa1@p.com",
			Project: "p",
		},
	}, "something went wrong"))
}

func newMockClient(t *testing.T) *mockClient {
	m := &mockClient{}
	t.Cleanup(func() {
		m.AssertExpectations(t)
	})
	return m
}

// mock implementation of slackClient
type mockClient struct {
	mock.Mock
}

func (f *mockClient) PostWebhook(message *slack.WebhookMessage) error {
	args := f.Called(message)
	return args.Error(0)
}
