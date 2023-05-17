package slack

import (
	"github.com/broadinstitute/yale/internal/yale/logs"
	"github.com/slack-go/slack"
)

const WebhookEnvVar = "YALE_SLACK_WEBHOOK_URL"

// slackClient is an interface for sending messages via slack webhooks
// it exists to allow for mocking in tests
type slackClient interface {
	PostWebhook(message *slack.WebhookMessage) error
}

func newSlackClient(webhookUrl string) slackClient {
	if len(webhookUrl) == 0 {
		logs.Warn.Printf("Slack notifications are disabled; set `%s` to enable Slack notifications for Yale", WebhookEnvVar)
	}
	return realClient{webhookUrl: webhookUrl}
}

type realClient struct {
	webhookUrl string
}

func (r realClient) PostWebhook(message *slack.WebhookMessage) error {
	if len(r.webhookUrl) == 0 {
		return nil
	}
	return slack.PostWebhook(r.webhookUrl, message)
}
